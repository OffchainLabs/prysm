package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

func (c *beaconApiValidatorClient) subscribeCommitteeSubnets(ctx context.Context, in *ethpb.CommitteeSubnetsSubscribeRequest, validatorIndices []primitives.ValidatorIndex, committeesAtSlot []uint64) error {
	if in == nil {
		return errors.New("committee subnets subscribe request is nil")
	}

	if len(in.CommitteeIds) != len(in.Slots) || len(in.CommitteeIds) != len(in.IsAggregator) ||
		len(in.CommitteeIds) != len(validatorIndices) || len(in.CommitteeIds) != len(committeesAtSlot) {
		return errors.New("arrays `in.CommitteeIds`, `in.Slots`, `in.IsAggregator`, `validatorIndices` and `committeesAtSlot` don't have the same length")
	}

	jsonCommitteeSubscriptions := make([]*structs.BeaconCommitteeSubscription, len(in.CommitteeIds))
	for index := range in.CommitteeIds {
		jsonCommitteeSubscriptions[index] = &structs.BeaconCommitteeSubscription{
			CommitteeIndex:   strconv.FormatUint(uint64(in.CommitteeIds[index]), 10),
			CommitteesAtSlot: strconv.FormatUint(committeesAtSlot[index], 10),
			Slot:             strconv.FormatUint(uint64(in.Slots[index]), 10),
			IsAggregator:     in.IsAggregator[index],
			ValidatorIndex:   strconv.FormatUint(uint64(validatorIndices[index]), 10),
		}
	}

	committeeSubscriptionsBytes, err := json.Marshal(jsonCommitteeSubscriptions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal committees subscriptions")
	}

	return c.handler.Post(
		ctx,
		"/eth/v1/validator/beacon_committee_subscriptions",
		nil,
		bytes.NewBuffer(committeeSubscriptionsBytes),
		nil,
	)
}
