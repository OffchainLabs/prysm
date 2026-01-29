package gloas

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// ProcessBuilderPendingPayments processes the builder pending payments from the previous epoch.
//
//	<spec fn="process_builder_pending_payments" fork="gloas" hash="10da48dd">
//	def process_builder_pending_payments(state: BeaconState) -> None:
//	    """
//	    Processes the builder pending payments from the previous epoch.
//	    """
//	    quorum = get_builder_payment_quorum_threshold(state)
//	    for payment in state.builder_pending_payments[:SLOTS_PER_EPOCH]:
//	        if payment.weight >= quorum:
//	            state.builder_pending_withdrawals.append(payment.withdrawal)
//
//	    old_payments = state.builder_pending_payments[SLOTS_PER_EPOCH:]
//	    new_payments = [BuilderPendingPayment() for _ in range(SLOTS_PER_EPOCH)]
//	    state.builder_pending_payments = old_payments + new_payments
//	</spec>
//
func ProcessBuilderPendingPayments(state state.BeaconState) error {
	quorum, err := builderQuorumThreshold(state)
	if err != nil {
		return errors.Wrap(err, "could not compute builder payment quorum threshold")
	}

	payments, err := state.BuilderPendingPayments()
	if err != nil {
		return errors.Wrap(err, "could not get builder pending payments")
	}

	slotsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch)
	var withdrawals []*ethpb.BuilderPendingWithdrawal
	for _, payment := range payments[:slotsPerEpoch] {
		if quorum > payment.Weight {
			continue
		}
		withdrawals = append(withdrawals, payment.Withdrawal)
	}

	if err := state.AppendBuilderPendingWithdrawals(withdrawals); err != nil {
		return errors.Wrap(err, "could not append builder pending withdrawals")
	}

	if err := state.RotateBuilderPendingPayments(); err != nil {
		return errors.Wrap(err, "could not rotate builder pending payments")
	}

	return nil
}

// builderQuorumThreshold calculates the quorum threshold for builder payments.
//
//	<spec fn="get_builder_payment_quorum_threshold" fork="gloas" hash="a64b7ffb">
//	def get_builder_payment_quorum_threshold(state: BeaconState) -> uint64:
//	    """
//	    Calculate the quorum threshold for builder payments.
//	    """
//	    per_slot_balance = get_total_active_balance(state) // SLOTS_PER_EPOCH
//	    quorum = per_slot_balance * BUILDER_PAYMENT_THRESHOLD_NUMERATOR
//	    return uint64(quorum // BUILDER_PAYMENT_THRESHOLD_DENOMINATOR)
//	</spec>
func builderQuorumThreshold(state state.ReadOnlyBeaconState) (primitives.Gwei, error) {
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
