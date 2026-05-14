package state_native

import (
	"bytes"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// DepositBalanceToConsume is a non-mutating call to the beacon state which returns the value of the
// deposit balance to consume field. This method requires access to the RLock on the state and only
// applies in electra or later.
func (b *BeaconState) DepositBalanceToConsume() (primitives.Gwei, error) {
	if b.version < version.Electra {
		return 0, errNotSupported("DepositBalanceToConsume", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.depositBalanceToConsume, nil
}

// PendingDeposits is a non-mutating call to the beacon state which returns a deep copy of
// the pending balance deposit slice. This method requires access to the RLock on the state and
// only applies in electra or later.
func (b *BeaconState) PendingDeposits() ([]*ethpb.PendingDeposit, error) {
	if b.version < version.Electra {
		return nil, errNotSupported("PendingDeposits", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.pendingDepositsVal(), nil
}

// IsPendingValidator checks whether a pending deposit with a valid signature exists for the
// given pubkey. This method requires access to the RLock on the state and only applies in
// electra or later.
//
// <spec fn="is_pending_validator" fork="gloas" hash="9b409bab">
// def is_pending_validator(state: BeaconState, pubkey: BLSPubkey) -> bool:
//
//	"""
//	Check if a pending deposit with a valid signature is in the queue for the given pubkey.
//	"""
//	for pending_deposit in state.pending_deposits:
//	    if pending_deposit.pubkey != pubkey:
//	        continue
//	    if is_valid_deposit_signature(
//	        pending_deposit.pubkey,
//	        pending_deposit.withdrawal_credentials,
//	        pending_deposit.amount,
//	        pending_deposit.signature,
//	    ):
//	        return True
//	return False
//
// </spec>
func (b *BeaconState) IsPendingValidator(pubkey []byte) (bool, error) {
	if b.version < version.Electra {
		return false, errNotSupported("IsPendingValidator", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	for _, deposit := range b.pendingDeposits {
		if deposit == nil {
			continue
		}
		if !bytes.Equal(deposit.PublicKey, pubkey) {
			continue
		}
		valid, err := helpers.IsValidDepositSignature(&ethpb.Deposit_Data{
			PublicKey:             deposit.PublicKey,
			WithdrawalCredentials: deposit.WithdrawalCredentials,
			Amount:                deposit.Amount,
			Signature:             deposit.Signature,
		})
		if err != nil {
			log.WithField("pubkey", fmt.Sprintf("%x", deposit.PublicKey)).WithError(err).Warn("Could not verify pending deposit signature")
			continue
		}
		if valid {
			return true, nil
		}
	}
	return false, nil
}

func (b *BeaconState) pendingDepositsVal() []*ethpb.PendingDeposit {
	if b.pendingDeposits == nil {
		return nil
	}

	return ethpb.CopySlice(b.pendingDeposits)
}
