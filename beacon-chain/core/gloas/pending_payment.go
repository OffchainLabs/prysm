package gloas

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/pkg/errors"
)

// ProcessBuilderPendingPayments processes the builder pending payments from the previous epoch.
// Spec v1.7.0-alpha.0 (pseudocode):
// process_builder_pending_payments(state: BeaconState) -> None:
//
//	quorum = get_builder_payment_quorum_threshold(state)
//	for payment in state.builder_pending_payments[:SLOTS_PER_EPOCH]:
//	    if payment.weight >= quorum:
//	        state.builder_pending_withdrawals.append(payment.withdrawal)
//	state.builder_pending_payments =
//	    state.builder_pending_payments[SLOTS_PER_EPOCH:]
//	    + [BuilderPendingPayment() for _ in range(SLOTS_PER_EPOCH)]
func ProcessBuilderPendingPayments(state state.BeaconState) error {
	quorum, err := builderQuorumThreshold(state)
	if err != nil {
		return errors.Wrap(err, "could not compute builder payment quorum threshold")
	}

	payments, err := state.BuilderPendingPaymentsNoCopy()
	if err != nil {
		return errors.Wrap(err, "could not get builder pending payments")
	}

	slotsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch)
	for i := range slotsPerEpoch {
		payment := payments[i]
		if quorum > payment.Weight {
			continue
		}

		if err := state.AppendBuilderPendingWithdrawal(payment.Withdrawal); err != nil {
			return errors.Wrapf(err, "could not append builder pending withdrawal %d", i)
		}
	}

	if err := state.RotateBuilderPendingPayments(); err != nil {
		return errors.Wrap(err, "could not rotate builder pending payments")
	}

	return nil
}

// builderQuorumThreshold calculates the quorum threshold for builder payments.
// Spec v1.7.0-alpha.0 (pseudocode):
// def get_builder_payment_quorum_threshold(state: BeaconState) -> uint64:
//
//	per_slot_balance = get_total_active_balance(state) // SLOTS_PER_EPOCH
//	quorum = per_slot_balance * BUILDER_PAYMENT_THRESHOLD_NUMERATOR
//	return uint64(quorum // BUILDER_PAYMENT_THRESHOLD_DENOMINATOR)
func builderQuorumThreshold(state state.BeaconState) (primitives.Gwei, error) {
	activeBalance, err := helpers.TotalActiveBalance(state)
	if err != nil {
		return 0, errors.Wrap(err, "could not get total active balance")
	}

	cfg := params.BeaconConfig()
	slotsPerEpoch := uint64(cfg.SlotsPerEpoch)
	numerator := cfg.BuilderPaymentThresholdNumerator
	denominator := cfg.BuilderPaymentThresholdDenominator

	activeBalancePerSlot := activeBalance / slotsPerEpoch
	quorum := (activeBalancePerSlot * numerator) / denominator
	return primitives.Gwei(quorum), nil
}
