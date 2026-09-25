package decoupled

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// Spec: BuilderPendingPayment()
func emptyBuilderPendingPayment() *ethpb.BuilderPendingPaymentDecoupled {
	return &ethpb.BuilderPendingPaymentDecoupled{
		AvailableParticipation:  make([]byte, fieldparams.AvailableCommitteeSize/8),
		TimelyHeadParticipation: make([]byte, fieldparams.AvailableCommitteeSize/8),
		Withdrawal:              &ethpb.BuilderPendingWithdrawal{FeeRecipient: make([]byte, fieldparams.FeeRecipientLength)},
	}
}

// Spec: process_builder_pending_payments. Legacy weight pays pre-fork claims, the seat popcount pays post-fork ones.
func ProcessBuilderPendingPayments(ctx context.Context, st state.BeaconState) error {
	cfg := params.BeaconConfig()
	payments, err := st.BuilderPendingPaymentsDecoupled()
	if err != nil {
		return err
	}
	legacyQuorum, err := gloas.BuilderQuorumThreshold(ctx, st)
	if err != nil {
		return err
	}
	slotsPerEpoch := uint64(cfg.SlotsPerEpoch)
	var due []*ethpb.BuilderPendingWithdrawal
	for _, p := range payments[:slotsPerEpoch] {
		hasLegacyQuorum := p.Weight > 0 && p.Weight >= legacyQuorum
		hasAvailableQuorum := p.AvailableParticipation.Count()*cfg.BuilderPaymentThresholdDenominator >= fieldparams.AvailableCommitteeSize*cfg.BuilderPaymentThresholdNumerator
		if (hasLegacyQuorum || hasAvailableQuorum) && p.Withdrawal != nil && p.Withdrawal.Amount > 0 {
			due = append(due, p.Withdrawal)
		}
	}
	if len(due) > 0 {
		if err := st.AppendBuilderPendingWithdrawals(due); err != nil {
			return err
		}
	}
	next := make([]*ethpb.BuilderPendingPaymentDecoupled, len(payments))
	copy(next, payments[slotsPerEpoch:])
	for i := slotsPerEpoch; i < uint64(len(next)); i++ {
		next[i] = emptyBuilderPendingPayment()
	}
	return st.SetBuilderPendingPaymentsDecoupled(next)
}
