package electra

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// Spec: compute_deposit_vrf_root
func depositVRFRootPQ(deposit *ethpb.PendingDeposit) [32]byte {
	if helpers.IsLegacyPubkey(deposit.PublicKey) {
		return helpers.LegacyVRFRoot(deposit.PublicKey)
	}
	return bytesutil.ToBytes32(deposit.Signature)
}

// Spec: apply_pending_deposit
func applyPendingDepositPQ(st state.BeaconState, deposit *ethpb.PendingDeposit) error {
	pubkey := helpers.DepositPubkeyPQ(deposit.PublicKey)
	if index, ok := st.ValidatorIndexByPubkey(pubkey); ok {
		return helpers.IncreaseBalance(st, index, deposit.Amount)
	}
	val, err := GetValidatorFromDeposit(pubkey[:], deposit.WithdrawalCredentials, deposit.Amount)
	if err != nil {
		return errors.Wrap(err, "could not get validator from deposit")
	}
	// A legacy key can never sign, so its validator is exited at once without using exit churn.
	if helpers.IsLegacyPubkey(deposit.PublicKey) {
		epoch := slots.ToEpoch(st.Slot())
		val.ExitEpoch = epoch
		val.WithdrawableEpoch = epoch + params.BeaconConfig().MinValidatorWithdrawabilityDelay
	}
	if err := st.AppendValidatorPQ(val, depositVRFRootPQ(deposit)); err != nil {
		return err
	}
	if err := st.AppendBalance(deposit.Amount); err != nil {
		return err
	}
	if err := st.AppendInactivityScore(0); err != nil {
		return err
	}
	if err := st.AppendPreviousParticipationBits(0); err != nil {
		return err
	}
	return st.AppendCurrentParticipationBits(0)
}

// Spec: process_pending_deposits
func processPendingDepositsPQ(st state.BeaconState, activeBalance primitives.Gwei) error {
	nextEpoch := slots.ToEpoch(st.Slot()) + 1
	depBalToConsume, err := st.DepositBalanceToConsume()
	if err != nil {
		return errors.Wrap(err, "could not get deposit balance to consume")
	}
	availableForProcessing := depBalToConsume + helpers.ActivationChurnLimitForVersion(st.Version(), activeBalance)
	finalizedSlot, err := slots.EpochStart(st.FinalizedCheckpoint().Epoch)
	if err != nil {
		return errors.Wrap(err, "could not get finalized slot")
	}
	pendingDeposits, err := st.PendingDeposits()
	if err != nil {
		return err
	}
	var (
		processedAmount     uint64
		nextDepositIndex    uint64
		isChurnLimitReached bool
		toPostpone          []*ethpb.PendingDeposit
	)
	for _, deposit := range pendingDeposits {
		if deposit.Slot > finalizedSlot || nextDepositIndex >= params.BeaconConfig().MaxPendingDepositsPerEpoch {
			break
		}
		var isExited, isWithdrawn bool
		if index, found := st.ValidatorIndexByPubkey(helpers.DepositPubkeyPQ(deposit.PublicKey)); found {
			val, err := st.ValidatorAtIndexReadOnly(index)
			if err != nil {
				return errors.Wrap(err, "could not get validator")
			}
			isExited = val.ExitEpoch() < params.BeaconConfig().FarFutureEpoch
			isWithdrawn = val.WithdrawableEpoch() < nextEpoch
		}
		switch {
		case isWithdrawn || helpers.IsLegacyPubkey(deposit.PublicKey):
			if err := applyPendingDepositPQ(st, deposit); err != nil {
				return errors.Wrap(err, "could not apply pending deposit")
			}
		case isExited:
			toPostpone = append(toPostpone, deposit)
		default:
			isChurnLimitReached = primitives.Gwei(processedAmount+deposit.Amount) > availableForProcessing
			if isChurnLimitReached {
				break
			}
			processedAmount += deposit.Amount
			if err := applyPendingDepositPQ(st, deposit); err != nil {
				return errors.Wrap(err, "could not apply pending deposit")
			}
		}
		if isChurnLimitReached {
			break
		}
		nextDepositIndex++
	}
	if err := st.SetPendingDeposits(append(pendingDeposits[nextDepositIndex:], toPostpone...)); err != nil {
		return errors.Wrap(err, "could not set pending deposits")
	}
	if isChurnLimitReached {
		return st.SetDepositBalanceToConsume(availableForProcessing - primitives.Gwei(processedAmount))
	}
	return st.SetDepositBalanceToConsume(0)
}
