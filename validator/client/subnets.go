package client

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/pkg/errors"
)

// subscribeToSubnets iterates through each validator duty, signs each slot, and asks beacon node
// to eagerly subscribe to subnets so that the aggregator has attestations to aggregate.
func (v *validator) subscribeToSubnets(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "validator.subscribeToSubnets")
	defer span.End()

	v.dutiesLock.RLock()
	currentDuties := v.duties.CurrentEpochDuties()
	nextDuties := v.duties.NextEpochDuties()
	v.dutiesLock.RUnlock()

	subscribeSlots := make([]primitives.Slot, 0, len(currentDuties)+len(nextDuties))
	subscribeCommitteeIndices := make([]primitives.CommitteeIndex, 0, len(currentDuties)+len(nextDuties))
	subscribeIsAggregator := make([]bool, 0, len(currentDuties)+len(nextDuties))
	activeDuties := make([]*ethpb.ValidatorDuty, 0, len(currentDuties)+len(nextDuties))
	alreadySubscribed := make(map[[64]byte]bool)

	if v.distributed {
		if err := v.aggregatedSelectionProofs(ctx, currentDuties); err != nil {
			return errors.Wrap(err, "could not get aggregated selection proofs")
		}
	}

	allDuties := append(currentDuties, nextDuties...)
	for _, duty := range allDuties {
		if duty.Status != ethpb.ValidatorStatus_ACTIVE && duty.Status != ethpb.ValidatorStatus_EXITING {
			continue
		}
		alreadySubscribedKey := validatorSubnetSubscriptionKey(duty.Slot, duty.CommitteeIndex)
		if _, ok := alreadySubscribed[alreadySubscribedKey]; ok {
			continue
		}

		aggregator, err := v.isAggregator(ctx, duty.CommitteeLength, duty.Slot, duty.Pubkey, duty.ValidatorIndex)
		if err != nil {
			return errors.Wrap(err, "could not check if a validator is an aggregator")
		}
		if aggregator {
			alreadySubscribed[alreadySubscribedKey] = true
		}

		subscribeSlots = append(subscribeSlots, duty.Slot)
		subscribeCommitteeIndices = append(subscribeCommitteeIndices, duty.CommitteeIndex)
		subscribeIsAggregator = append(subscribeIsAggregator, aggregator)
		activeDuties = append(activeDuties, dutyViewToProto(duty))
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

func (v *validator) aggregatedSelectionProofs(ctx context.Context, currentDuties []*attesterDutyView) error {
	ctx, span := trace.StartSpan(ctx, "validator.aggregatedSelectionProofs")
	defer span.End()

	v.attSelectionLock.Lock()
	defer v.attSelectionLock.Unlock()

	v.attSelections = make(map[attSelectionKey]iface.BeaconCommitteeSelection)

	var req []iface.BeaconCommitteeSelection
	for _, duty := range currentDuties {
		if duty.Status != ethpb.ValidatorStatus_ACTIVE && duty.Status != ethpb.ValidatorStatus_EXITING {
			continue
		}

		slotSig, err := v.signSlotWithSelectionProof(ctx, duty.Pubkey, duty.Slot)
		if err != nil {
			return err
		}

		req = append(req, iface.BeaconCommitteeSelection{
			SelectionProof: slotSig,
			Slot:           duty.Slot,
			ValidatorIndex: duty.ValidatorIndex,
		})
	}

	resp, err := v.validatorClient.AggregatedSelections(ctx, req)
	if err != nil {
		return err
	}

	for _, s := range resp {
		v.attSelections[attSelectionKey{
			slot:  s.Slot,
			index: s.ValidatorIndex,
		}] = s
	}

	return nil
}

func validatorSubnetSubscriptionKey(slot primitives.Slot, committeeIndex primitives.CommitteeIndex) [64]byte {
	return bytesutil.ToBytes64(append(bytesutil.Bytes32(uint64(slot)), bytesutil.Bytes32(uint64(committeeIndex))...))
}
