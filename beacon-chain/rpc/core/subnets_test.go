package core

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/pkg/errors"
)

type fakeHeadFetcher struct {
	count uint64
	err   error
	calls int
}

func (f *fakeHeadFetcher) HeadValidatorsIndices(_ context.Context, _ primitives.Epoch) ([]primitives.ValidatorIndex, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return make([]primitives.ValidatorIndex, f.count), nil
}

func TestComputeAndCacheCommitteeSubnets_UsesCommitteesAtSlot(t *testing.T) {
	cache.SubnetIDs.EmptyAllCaches()

	slot := primitives.Slot(8001)
	subs := []SubnetSubscription{
		{Slot: slot, CommitteeIndex: 2, IsAggregator: true, CommitteesAtSlot: 3},
	}
	fetcher := &fakeHeadFetcher{}
	require.NoError(t, ComputeAndCacheCommitteeSubnets(t.Context(), fetcher, subs))
	// committees_at_slot supplied: the head-state count must not be fetched.
	assert.Equal(t, 0, fetcher.calls)

	want := helpers.ComputeSubnetForCommitteesPerSlot(3, 2, slot)
	att := cache.SubnetIDs.GetAttesterSubnetIDs(slot)
	require.Equal(t, 1, len(att))
	assert.Equal(t, want, att[0])
	agg := cache.SubnetIDs.GetAggregatorSubnetIDs(slot)
	require.Equal(t, 1, len(agg))
	assert.Equal(t, want, agg[0])
}

func TestComputeAndCacheCommitteeSubnets_FallbackFetchesOncePerEpoch(t *testing.T) {
	cache.SubnetIDs.EmptyAllCaches()

	slotsPerEpoch := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	epochStart := 300 * slotsPerEpoch
	subs := []SubnetSubscription{
		{Slot: epochStart, CommitteeIndex: 0},
		{Slot: epochStart + 1, CommitteeIndex: 0},             // same epoch as above
		{Slot: epochStart + slotsPerEpoch, CommitteeIndex: 0}, // next epoch
	}
	fetcher := &fakeHeadFetcher{count: 256}
	require.NoError(t, ComputeAndCacheCommitteeSubnets(t.Context(), fetcher, subs))
	// One lookup per distinct epoch, not per subscription.
	assert.Equal(t, 2, fetcher.calls)
}

func TestComputeAndCacheCommitteeSubnets_FallbackError(t *testing.T) {
	cache.SubnetIDs.EmptyAllCaches()

	subs := []SubnetSubscription{{Slot: 8200, CommitteeIndex: 0}}
	fetcher := &fakeHeadFetcher{err: errors.New("boom")}
	err := ComputeAndCacheCommitteeSubnets(t.Context(), fetcher, subs)
	require.ErrorContains(t, "boom", err)
}
