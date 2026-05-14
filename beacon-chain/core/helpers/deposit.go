package helpers

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/contracts/deposit"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ActivateValidatorWithEffectiveBalance updates validator's effective balance, and if it's above MaxEffectiveBalance, validator becomes active in genesis.
func ActivateValidatorWithEffectiveBalance(beaconState state.BeaconState, deposits []*ethpb.Deposit) (state.BeaconState, error) {
	for _, d := range deposits {
		pubkey := d.Data.PublicKey
		index, ok := beaconState.ValidatorIndexByPubkey(bytesutil.ToBytes48(pubkey))
		// In the event of the pubkey not existing, we continue processing the other
		// deposits.
		if !ok {
			continue
		}
		balance, err := beaconState.BalanceAtIndex(index)
		if err != nil {
			return nil, err
		}
		validator, err := beaconState.ValidatorAtIndex(index)
		if err != nil {
			return nil, err
		}
		validator.EffectiveBalance = min(balance-balance%params.BeaconConfig().EffectiveBalanceIncrement, params.BeaconConfig().MaxEffectiveBalance)
		if validator.EffectiveBalance ==
			params.BeaconConfig().MaxEffectiveBalance {
			validator.ActivationEligibilityEpoch = 0
			validator.ActivationEpoch = 0
		}
		if err := beaconState.UpdateValidatorAtIndex(index, validator); err != nil {
			return nil, err
		}
	}
	return beaconState, nil
}

// BatchVerifyDepositsSignatures batch verifies deposit signatures.
func BatchVerifyDepositsSignatures(ctx context.Context, deposits []*ethpb.Deposit) (bool, error) {
	var err error
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	if err != nil {
		return false, err
	}

	if err := verifyDepositDataWithDomain(ctx, deposits, domain); err != nil {
		log.WithError(err).Debug("Failed to batch verify deposits signatures, will try individual verify")
		return false, nil
	}
	return true, nil
}

// BatchVerifyPendingDepositsSignatures batch verifies pending deposit signatures.
func BatchVerifyPendingDepositsSignatures(ctx context.Context, deposits []*ethpb.PendingDeposit) (bool, error) {
	var err error
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	if err != nil {
		return false, err
	}

	if err := verifyPendingDepositDataWithDomain(ctx, deposits, domain); err != nil {
		log.WithError(err).Debug("Failed to batch verify deposits signatures, will try individual verify")
		return false, nil
	}
	return true, nil
}

// BatchVerifyPendingDepositSignatures returns a per-deposit validity slice; falls back to divide-and-conquer on batch failure.
// Hits the BuilderOnboardingSig cache first so deposits verified pre-fork don't redo BLS work at the Gloas upgrade.
func BatchVerifyPendingDepositSignatures(ctx context.Context, deposits []*ethpb.PendingDeposit) ([]bool, error) {
	if len(deposits) == 0 {
		return nil, nil
	}
	lookupStart := time.Now()
	valid := make([]bool, len(deposits))
	keys := make([][32]byte, len(deposits))
	var missIdx []int
	var missDeposits []*ethpb.PendingDeposit
	for i, d := range deposits {
		keys[i] = cache.PendingDepositKey(d)
		if v, ok := cache.BuilderOnboardingSig.Get(keys[i]); ok {
			valid[i] = v
			continue
		}
		missIdx = append(missIdx, i)
		missDeposits = append(missDeposits, d)
	}
	lookupDur := time.Since(lookupStart)
	if len(missDeposits) == 0 {
		log.WithFields(logrus.Fields{
			"deposits": len(deposits),
			"hits":     len(deposits),
			"misses":   0,
			"lookup":   lookupDur,
		}).Info("Pending deposit batch verify: all cache hits")
		return valid, nil
	}
	verifyStart := time.Now()
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	if err != nil {
		return nil, err
	}
	missValid := make([]bool, len(missDeposits))
	if err := verifyPendingDepositsDC(ctx, missDeposits, domain, missValid); err != nil {
		return nil, err
	}
	validCount := 0
	for k, i := range missIdx {
		valid[i] = missValid[k]
		cache.BuilderOnboardingSig.Put(keys[i], missValid[k])
		if missValid[k] {
			validCount++
		}
	}
	log.WithFields(logrus.Fields{
		"deposits":  len(deposits),
		"hits":      len(deposits) - len(missDeposits),
		"misses":    len(missDeposits),
		"missValid": validCount,
		"lookup":    lookupDur,
		"verifyBls": time.Since(verifyStart),
	}).Info("Pending deposit batch verify")
	return valid, nil
}

func verifyPendingDepositsDC(ctx context.Context, deps []*ethpb.PendingDeposit, domain []byte, out []bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(deps) == 0 {
		return nil
	}
	if err := verifyPendingDepositDataWithDomain(ctx, deps, domain); err == nil {
		for i := range out {
			out[i] = true
		}
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(deps) == 1 {
		out[0] = false
		return nil
	}
	const fanout = 8
	chunk := (len(deps) + fanout - 1) / fanout
	for i := 0; i < len(deps); i += chunk {
		end := min(i+chunk, len(deps))
		if err := verifyPendingDepositsDC(ctx, deps[i:end], domain, out[i:end]); err != nil {
			return err
		}
	}
	return nil
}

// BatchVerifyDepositRequestSignatures returns a per-request validity slice; falls back to divide-and-conquer on batch failure.
func BatchVerifyDepositRequestSignatures(ctx context.Context, requests []*enginev1.DepositRequest) ([]bool, error) {
	if len(requests) == 0 {
		return nil, nil
	}
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	if err != nil {
		return nil, err
	}
	valid := make([]bool, len(requests))
	if err := verifyDepositRequestsDC(ctx, requests, domain, valid); err != nil {
		return nil, err
	}
	return valid, nil
}

func verifyDepositRequestsDC(ctx context.Context, reqs []*enginev1.DepositRequest, domain []byte, out []bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(reqs) == 0 {
		return nil
	}
	if err := verifyDepositRequestDataWithDomain(ctx, reqs, domain); err == nil {
		for i := range out {
			out[i] = true
		}
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(reqs) == 1 {
		out[0] = false
		return nil
	}
	const fanout = 8
	chunk := (len(reqs) + fanout - 1) / fanout
	for i := 0; i < len(reqs); i += chunk {
		end := min(i+chunk, len(reqs))
		if err := verifyDepositRequestsDC(ctx, reqs[i:end], domain, out[i:end]); err != nil {
			return err
		}
	}
	return nil
}

// IsValidDepositSignature returns whether deposit_data is valid
// def is_valid_deposit_signature(pubkey: BLSPubkey, withdrawal_credentials: Bytes32, amount: uint64, signature: BLSSignature) -> bool:
//
//	deposit_message = DepositMessage( pubkey=pubkey, withdrawal_credentials=withdrawal_credentials, amount=amount, )
//	domain = compute_domain(DOMAIN_DEPOSIT)  # Fork-agnostic domain since deposits are valid across forks
//	signing_root = compute_signing_root(deposit_message, domain)
//	return bls.Verify(pubkey, signing_root, signature)
func IsValidDepositSignature(data *ethpb.Deposit_Data) (bool, error) {
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	if err != nil {
		return false, err
	}
	if err := verifyDepositDataSigningRoot(data, domain); err != nil {
		// Ignore this error as in the spec pseudo code.
		log.WithError(err).Debug("Skipping deposit: could not verify deposit data signature")
		return false, nil
	}
	return true, nil
}

// VerifyDeposit verifies the deposit data and signature given the beacon state and deposit information
func VerifyDeposit(beaconState state.ReadOnlyBeaconState, deposit *ethpb.Deposit) error {
	// Verify Merkle proof of deposit and deposit trie root.
	if deposit == nil || deposit.Data == nil {
		return errors.New("received nil deposit or nil deposit data")
	}
	eth1Data := beaconState.Eth1Data()
	if eth1Data == nil {
		return errors.New("received nil eth1data in the beacon state")
	}

	receiptRoot := eth1Data.DepositRoot
	leaf, err := deposit.Data.HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not tree hash deposit data")
	}
	if ok := trie.VerifyMerkleProofWithDepth(
		receiptRoot,
		leaf[:],
		beaconState.Eth1DepositIndex(),
		deposit.Proof,
		params.BeaconConfig().DepositContractTreeDepth,
	); !ok {
		return fmt.Errorf(
			"deposit merkle branch of deposit root did not verify for root: %#x",
			receiptRoot,
		)
	}

	return nil
}

func verifyDepositDataSigningRoot(obj *ethpb.Deposit_Data, domain []byte) error {
	return deposit.VerifyDepositSignature(obj, domain)
}

func verifyDepositDataWithDomain(ctx context.Context, deps []*ethpb.Deposit, domain []byte) error {
	if len(deps) == 0 {
		return nil
	}
	pubkeyBytes := make([][]byte, len(deps))
	for i, dep := range deps {
		if dep == nil || dep.Data == nil {
			return errors.New("nil deposit")
		}
		pubkeyBytes[i] = dep.Data.PublicKey
	}
	pks, err := bls.MultiplePublicKeysFromBytes(pubkeyBytes)
	if err != nil {
		return err
	}
	sigs := make([][]byte, len(deps))
	msgs := make([][32]byte, len(deps))
	for i, dep := range deps {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		sigs[i] = dep.Data.Signature
		sr, err := signing.ComputeSigningRoot(&ethpb.DepositMessage{
			PublicKey:             dep.Data.PublicKey,
			WithdrawalCredentials: dep.Data.WithdrawalCredentials,
			Amount:                dep.Data.Amount,
		}, domain)
		if err != nil {
			return err
		}
		msgs[i] = sr
	}
	verify, err := bls.VerifyMultipleSignatures(sigs, msgs, pks)
	if err != nil {
		return errors.Errorf("could not verify multiple signatures: %v", err)
	}
	if !verify {
		return errors.New("one or more deposit signatures did not verify")
	}
	return nil
}

func verifyDepositRequestDataWithDomain(ctx context.Context, reqs []*enginev1.DepositRequest, domain []byte) error {
	if len(reqs) == 0 {
		return nil
	}
	pubkeyBytes := make([][]byte, len(reqs))
	for i, req := range reqs {
		if req == nil {
			return errors.New("nil deposit request")
		}
		pubkeyBytes[i] = req.Pubkey
	}
	pks, err := bls.MultiplePublicKeysFromBytes(pubkeyBytes)
	if err != nil {
		return err
	}
	sigs := make([][]byte, len(reqs))
	msgs := make([][32]byte, len(reqs))
	for i, req := range reqs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		sigs[i] = req.Signature
		sr, err := signing.ComputeSigningRoot(&ethpb.DepositMessage{
			PublicKey:             req.Pubkey,
			WithdrawalCredentials: req.WithdrawalCredentials,
			Amount:                req.Amount,
		}, domain)
		if err != nil {
			return err
		}
		msgs[i] = sr
	}
	verify, err := bls.VerifyMultipleSignatures(sigs, msgs, pks)
	if err != nil {
		return errors.Errorf("could not verify multiple signatures: %v", err)
	}
	if !verify {
		return errors.New("one or more deposit signatures did not verify")
	}
	return nil
}

func verifyPendingDepositDataWithDomain(ctx context.Context, deps []*ethpb.PendingDeposit, domain []byte) error {
	if len(deps) == 0 {
		return nil
	}
	pubkeyBytes := make([][]byte, len(deps))
	for i, dep := range deps {
		if dep == nil {
			return errors.New("nil deposit")
		}
		pubkeyBytes[i] = dep.PublicKey
	}
	pks, err := bls.MultiplePublicKeysFromBytes(pubkeyBytes)
	if err != nil {
		return err
	}
	sigs := make([][]byte, len(deps))
	msgs := make([][32]byte, len(deps))
	for i, dep := range deps {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		sigs[i] = dep.Signature
		sr, err := signing.ComputeSigningRoot(&ethpb.DepositMessage{
			PublicKey:             dep.PublicKey,
			WithdrawalCredentials: dep.WithdrawalCredentials,
			Amount:                dep.Amount,
		}, domain)
		if err != nil {
			return err
		}
		msgs[i] = sr
	}
	verify, err := bls.VerifyMultipleSignatures(sigs, msgs, pks)
	if err != nil {
		return errors.Errorf("could not verify multiple signatures: %v", err)
	}
	if !verify {
		return errors.New("one or more deposit signatures did not verify")
	}
	return nil
}
