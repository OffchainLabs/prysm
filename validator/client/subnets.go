package client

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// subscribeToSubnets iterates through each validator duty, signs each slot, and asks beacon node
// to eagerly subscribe to subnets so that the aggregator has attestations to aggregate.
func (v *validator) subscribeToSubnets(ctx context.Context, duties *ethpb.ValidatorDutiesContainer) error {
	ctx, span := trace.StartSpan(ctx, "validator.subscribeToSubnets")
	defer span.End()

	total := len(duties.CurrentEpochDuties) + len(duties.NextEpochDuties)
	subscribeSlots := make([]primitives.Slot, 0, total)
	subscribeCommitteeIndices := make([]primitives.CommitteeIndex, 0, total)
	subscribeIsAggregator := make([]bool, 0, total)
	activeDuties := make([]*ethpb.ValidatorDuty, 0, total)

	if err := v.aggSelector.RefreshSelectionProofs(ctx); err != nil {
		return errors.Wrap(err, "could not prepare aggregated selection proofs")
	}

	for _, set := range [][]*ethpb.ValidatorDuty{duties.CurrentEpochDuties, duties.NextEpochDuties} {
		for _, duty := range set {
			if duty.Status != ethpb.ValidatorStatus_ACTIVE && duty.Status != ethpb.ValidatorStatus_EXITING {
				continue
			}
			isAgg, err := v.isAggregator(ctx, duty.CommitteeLength, duty.AttesterSlot, bytesutil.ToBytes48(duty.PublicKey))
			if err != nil {
				return errors.Wrap(err, "could not check if a validator is an aggregator")
			}
			subscribeSlots = append(subscribeSlots, duty.AttesterSlot)
			subscribeCommitteeIndices = append(subscribeCommitteeIndices, duty.CommitteeIndex)
			subscribeIsAggregator = append(subscribeIsAggregator, isAgg)
			activeDuties = append(activeDuties, duty)
		}
	}

	_, err := v.validatorClient.SubscribeCommitteeSubnets(ctx,
		&ethpb.CommitteeSubnetsSubscribeRequest{
			Slots:        subscribeSlots,
			CommitteeIds: subscribeCommitteeIndices,
			IsAggregator: subscribeIsAggregator,
		},
		activeDuties,
	)

	return err
}

func validatorSubnetSubscriptionKey(slot primitives.Slot, committeeIndex primitives.CommitteeIndex) [64]byte {
	return bytesutil.ToBytes64(append(bytesutil.Bytes32(uint64(slot)), bytesutil.Bytes32(uint64(committeeIndex))...))
}
