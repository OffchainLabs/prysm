package core

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// HeadValidatorIndicesFetcher returns the active validator indices at an epoch
// from head state; the subnet helper uses its count for the committees-per-slot
// fallback.
type HeadValidatorIndicesFetcher interface {
	HeadValidatorsIndices(ctx context.Context, epoch primitives.Epoch) ([]primitives.ValidatorIndex, error)
}

// SubnetSubscription is a single committee-subnet subscription item, shared by
// the gRPC and REST SubscribeCommitteeSubnets handlers.
type SubnetSubscription struct {
	Slot             primitives.Slot
	CommitteeIndex   primitives.CommitteeIndex
	IsAggregator     bool
	CommitteesAtSlot uint64
}

// ComputeAndCacheCommitteeSubnets records the attester (and aggregator) subnet
// for each subscription. CommitteesAtSlot yields the subnet directly; when it is
// zero the committee count comes from the head active-validator count, looked up
// once per epoch.
func ComputeAndCacheCommitteeSubnets(ctx context.Context, headFetcher HeadValidatorIndicesFetcher, subs []SubnetSubscription) error {
	var currValsLen uint64
	var currEpoch primitives.Epoch
	haveValsLen := false
	for _, sub := range subs {
		var subnet uint64
		if sub.CommitteesAtSlot > 0 {
			subnet = helpers.ComputeSubnetForCommitteesPerSlot(sub.CommitteesAtSlot, sub.CommitteeIndex, sub.Slot)
		} else {
			epoch := slots.ToEpoch(sub.Slot)
			if !haveValsLen || currEpoch != epoch {
				vals, err := headFetcher.HeadValidatorsIndices(ctx, epoch)
				if err != nil {
					return err
				}
				currValsLen, currEpoch, haveValsLen = uint64(len(vals)), epoch, true
			}
			subnet = helpers.ComputeSubnetFromCommitteeAndSlot(currValsLen, sub.CommitteeIndex, sub.Slot)
		}
		cache.SubnetIDs.AddAttesterSubnetID(sub.Slot, subnet)
		if sub.IsAggregator {
			cache.SubnetIDs.AddAggregatorSubnetID(sub.Slot, subnet)
		}
	}
	return nil
}
