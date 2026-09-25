package decoupled

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// Spec: BuilderPendingPayment()
func emptyBuilderPendingPayment() *ethpb.BuilderPendingPaymentDecoupled {
	return &ethpb.BuilderPendingPaymentDecoupled{
		AvailableParticipation:  make([]byte, fieldparams.AvailableCommitteeSize/8),
		TimelyHeadParticipation: make([]byte, fieldparams.AvailableCommitteeSize/8),
		Withdrawal:              &ethpb.BuilderPendingWithdrawal{FeeRecipient: make([]byte, fieldparams.FeeRecipientLength)},
	}
}

// Spec: process_proposer_slashing, the payment step. The claim is cancelled but both seat bitmaps stay as replay protection.
func CancelBuilderPendingPayment(st state.BeaconState, header *ethpb.BeaconBlockHeader) error {
	cfg := params.BeaconConfig()
	proposalEpoch := slots.ToEpoch(header.Slot)
	currentEpoch := time.CurrentEpoch(st)
	var index primitives.Slot
	switch {
	case proposalEpoch == currentEpoch:
		index = cfg.SlotsPerEpoch + header.Slot%cfg.SlotsPerEpoch
	case proposalEpoch+1 == currentEpoch:
		index = header.Slot % cfg.SlotsPerEpoch
	default:
		return nil
	}
	payments, err := st.BuilderPendingPaymentsDecoupled()
	if err != nil {
		return err
	}
	p := payments[index]
	payments[index] = &ethpb.BuilderPendingPaymentDecoupled{
		AvailableParticipation:  p.AvailableParticipation,
		TimelyHeadParticipation: p.TimelyHeadParticipation,
		Withdrawal:              &ethpb.BuilderPendingWithdrawal{FeeRecipient: make([]byte, fieldparams.FeeRecipientLength)},
	}
	return st.SetBuilderPendingPaymentsDecoupled(payments)
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
