package blocks

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/container/trie"
	"github.com/prysmaticlabs/prysm/v5/contracts/deposit"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/math"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
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
		validator.EffectiveBalance = math.Min(balance-balance%params.BeaconConfig().EffectiveBalanceIncrement, params.BeaconConfig().MaxEffectiveBalance)
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
	pks := make([]bls.PublicKey, len(deps))
	sigs := make([][]byte, len(deps))
	msgs := make([][32]byte, len(deps))
	for i, dep := range deps {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if dep == nil || dep.Data == nil {
			return errors.New("nil deposit")
		}
		dpk, err := bls.PublicKeyFromBytes(dep.Data.PublicKey)
		if err != nil {
			return err
		}
		pks[i] = dpk
		sigs[i] = dep.Data.Signature
		depositMessage := &ethpb.DepositMessage{
			PublicKey:             dep.Data.PublicKey,
			WithdrawalCredentials: dep.Data.WithdrawalCredentials,
			Amount:                dep.Data.Amount,
		}
		sr, err := signing.ComputeSigningRoot(depositMessage, domain)
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
	pks := make([]bls.PublicKey, len(deps))
	sigs := make([][]byte, len(deps))
	msgs := make([][32]byte, len(deps))
	for i, dep := range deps {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if dep == nil {
			return errors.New("nil deposit")
		}
		dpk, err := bls.PublicKeyFromBytes(dep.PublicKey)
		if err != nil {
			return err
		}
		pks[i] = dpk
		sigs[i] = dep.Signature
		depositMessage := &ethpb.DepositMessage{
			PublicKey:             dep.PublicKey,
			WithdrawalCredentials: dep.WithdrawalCredentials,
			Amount:                dep.Amount,
		}
		sr, err := signing.ComputeSigningRoot(depositMessage, domain)
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
