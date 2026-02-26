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
func (v *validator) subscribeToSubnets(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "validator.subscribeToSubnets")
	defer span.End()

	v.dutiesLock.RLock()
	ds := v.duties
	v.dutiesLock.RUnlock()

	currentDuties := ds.CurrentEpochDuties()
	nextDuties := ds.NextEpochDuties()

	totalDuties := len(currentDuties) + len(nextDuties)
	subscribeSlots := make([]primitives.Slot, 0, totalDuties)
	subscribeCommitteeIndices := make([]primitives.CommitteeIndex, 0, totalDuties)
	subscribeIsAggregator := make([]bool, 0, totalDuties)
	subscribeValidatorIndices := make([]primitives.ValidatorIndex, 0, totalDuties)
	subscribeCommitteesAtSlot := make([]uint64, 0, totalDuties)
	alreadySubscribed := make(map[[64]byte]bool)

	addDuty := func(duty *ethpb.AttesterDuty) error {
		alreadySubscribedKey := validatorSubnetSubscriptionKey(duty.Slot, duty.CommitteeIndex)
		if alreadySubscribed[alreadySubscribedKey] {
			return nil
		}

		pk := bytesutil.ToBytes48(duty.Pubkey)
		aggregator, err := v.isAggregator(ctx, duty.CommitteeLength, duty.Slot, pk, duty.ValidatorIndex)
		if err != nil {
			return errors.Wrap(err, "could not check if a validator is an aggregator")
		}
		if aggregator {
			alreadySubscribed[alreadySubscribedKey] = true
		}

		subscribeSlots = append(subscribeSlots, duty.Slot)
		subscribeCommitteeIndices = append(subscribeCommitteeIndices, duty.CommitteeIndex)
		subscribeIsAggregator = append(subscribeIsAggregator, aggregator)
		subscribeValidatorIndices = append(subscribeValidatorIndices, duty.ValidatorIndex)
		subscribeCommitteesAtSlot = append(subscribeCommitteesAtSlot, duty.CommitteesAtSlot)
		return nil
	}

	for _, duty := range currentDuties {
		if err := addDuty(duty); err != nil {
			return err
		}
	}
	for _, duty := range nextDuties {
		if err := addDuty(duty); err != nil {
			return err
		}
	}

	_, err := v.validatorClient.SubscribeCommitteeSubnets(ctx,
		&ethpb.CommitteeSubnetsSubscribeRequest{
			Slots:        subscribeSlots,
			CommitteeIds: subscribeCommitteeIndices,
			IsAggregator: subscribeIsAggregator,
		},
		subscribeValidatorIndices,
		subscribeCommitteesAtSlot,
	)

	return err
}

func validatorSubnetSubscriptionKey(slot primitives.Slot, committeeIndex primitives.CommitteeIndex) [64]byte {
	return bytesutil.ToBytes64(append(bytesutil.Bytes32(uint64(slot)), bytesutil.Bytes32(uint64(committeeIndex))...))
}
