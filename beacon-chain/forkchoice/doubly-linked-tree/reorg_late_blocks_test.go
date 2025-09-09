package doublylinkedtree

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestForkChoice_ShouldOverrideFCU(t *testing.T) {
	f := setup(0, 0)
	f.numActiveValidators = 640
	f.justifiedBalances = make([]uint64, f.numActiveValidators)
	for i := range f.justifiedBalances {
		f.justifiedBalances[i] = uint64(10)
		f.store.committeeWeight += uint64(10)
	}
	f.store.committeeWeight /= uint64(params.BeaconConfig().SlotsPerEpoch)
	ctx := t.Context()
	driftGenesisTime(f, 1, 0)
	st, blk, err := prepareForkchoiceState(ctx, 1, [32]byte{'a'}, [32]byte{}, [32]byte{'A'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))
	attesters := make([]uint64, f.numActiveValidators-64)
	for i := range attesters {
		attesters[i] = uint64(i + 64)
	}
	f.ProcessAttestation(ctx, attesters, blk.Root(), 0)

	orphanLateBlockFirstThreshold := time.Duration(params.BeaconConfig().SecondsPerSlot/params.BeaconConfig().IntervalsPerSlot) * time.Second
	driftGenesisTime(f, 2, orphanLateBlockFirstThreshold+time.Second)
	st, blk, err = prepareForkchoiceState(ctx, 2, [32]byte{'b'}, [32]byte{'a'}, [32]byte{'B'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))
	headRoot, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, blk.Root(), headRoot)
	t.Run("head is weak", func(t *testing.T) {
		require.Equal(t, true, f.ShouldOverrideFCU())

	})
	t.Run("head is nil", func(t *testing.T) {
		saved := f.store.headNode
		f.store.headNode = nil
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode = saved
	})
	t.Run("head is not from current slot", func(t *testing.T) {
		driftGenesisTime(f, 3, 0)
		require.Equal(t, false, f.ShouldOverrideFCU())
		driftGenesisTime(f, 2, orphanLateBlockFirstThreshold+time.Second)
	})
	t.Run("head is from epoch boundary", func(t *testing.T) {
		saved := f.store.headNode.slot
		driftGenesisTime(f, params.BeaconConfig().SlotsPerEpoch-1, 0)
		f.store.headNode.slot = params.BeaconConfig().SlotsPerEpoch - 1
		require.Equal(t, false, f.ShouldOverrideFCU())
		driftGenesisTime(f, 2, orphanLateBlockFirstThreshold+time.Second)
		f.store.headNode.slot = saved
	})
	t.Run("head is early", func(t *testing.T) {
		saved := f.store.headNode.timestamp
		f.store.headNode.timestamp = saved.Add(-2 * time.Second)
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode.timestamp = saved
	})
	t.Run("chain not finalizing", func(t *testing.T) {
		saved := f.store.headNode.slot
		f.store.headNode.slot = 97
		driftGenesisTime(f, 97, orphanLateBlockFirstThreshold+time.Second)
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode.slot = saved
		driftGenesisTime(f, 2, orphanLateBlockFirstThreshold+time.Second)
	})
	t.Run("Not single block reorg", func(t *testing.T) {
		saved := f.store.headNode.parent.slot
		f.store.headNode.parent.slot = 0
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode.parent.slot = saved
	})
	t.Run("parent is nil", func(t *testing.T) {
		saved := f.store.headNode.parent
		f.store.headNode.parent = nil
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode.parent = saved
	})
	t.Run("parent is weak early call", func(t *testing.T) {
		saved := f.store.headNode.parent.weight
		f.store.headNode.parent.weight = 0
		require.Equal(t, true, f.ShouldOverrideFCU())
		f.store.headNode.parent.weight = saved
	})
	t.Run("parent is weak late call", func(t *testing.T) {
		saved := f.store.headNode.parent.weight
		driftGenesisTime(f, 2, 11*time.Second)
		f.store.headNode.parent.weight = 0
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode.parent.weight = saved
		driftGenesisTime(f, 2, orphanLateBlockFirstThreshold+time.Second)
	})
	t.Run("Head is strong", func(t *testing.T) {
		f.store.headNode.weight = f.store.committeeWeight
		require.Equal(t, false, f.ShouldOverrideFCU())
	})
}

func TestForkChoice_GetProposerHead(t *testing.T) {
	f := setup(0, 0)
	f.numActiveValidators = 640
	f.justifiedBalances = make([]uint64, f.numActiveValidators)
	for i := range f.justifiedBalances {
		f.justifiedBalances[i] = uint64(10)
		f.store.committeeWeight += uint64(10)
	}
	f.store.committeeWeight /= uint64(params.BeaconConfig().SlotsPerEpoch)
	ctx := t.Context()
	driftGenesisTime(f, 1, 0)
	parentRoot := [32]byte{'a'}
	st, blk, err := prepareForkchoiceState(ctx, 1, parentRoot, [32]byte{}, [32]byte{'A'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))
	attesters := make([]uint64, f.numActiveValidators-64)
	for i := range attesters {
		attesters[i] = uint64(i + 64)
	}
	f.ProcessAttestation(ctx, attesters, blk.Root(), 0)

	driftGenesisTime(f, 3, 1*time.Second)
	childRoot := [32]byte{'b'}
	st, blk, err = prepareForkchoiceState(ctx, 2, childRoot, [32]byte{'a'}, [32]byte{'B'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))
	headRoot, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, blk.Root(), headRoot)
	orphanLateBlockFirstThreshold := params.BeaconConfig().SecondsPerSlot / params.BeaconConfig().IntervalsPerSlot
	f.store.headNode.timestamp.Add(-1 * time.Duration(params.BeaconConfig().SecondsPerSlot-orphanLateBlockFirstThreshold) * time.Second)
	t.Run("head is weak", func(t *testing.T) {
		require.Equal(t, parentRoot, f.GetProposerHead())
	})
	t.Run("head is nil", func(t *testing.T) {
		saved := f.store.headNode
		f.store.headNode = nil
		require.Equal(t, [32]byte{}, f.GetProposerHead())
		f.store.headNode = saved
	})
	t.Run("head is not from previous slot", func(t *testing.T) {
		driftGenesisTime(f, 4, 0)
		require.Equal(t, childRoot, f.GetProposerHead())
		driftGenesisTime(f, 3, 1*time.Second)
	})
	t.Run("head is from epoch boundary", func(t *testing.T) {
		saved := f.store.headNode.slot
		driftGenesisTime(f, params.BeaconConfig().SlotsPerEpoch, 0)
		f.store.headNode.slot = params.BeaconConfig().SlotsPerEpoch - 1
		require.Equal(t, childRoot, f.GetProposerHead())
		driftGenesisTime(f, 3, 1*time.Second)
		f.store.headNode.slot = saved
	})
	t.Run("head is early", func(t *testing.T) {
		saved := f.store.headNode.timestamp
		headTimeStamp := f.store.genesisTime.Add(time.Duration(uint64(f.store.headNode.slot)*params.BeaconConfig().SecondsPerSlot+1) * time.Second)
		f.store.headNode.timestamp = headTimeStamp
		require.Equal(t, childRoot, f.GetProposerHead())
		f.store.headNode.timestamp = saved
	})
	t.Run("chain not finalizing", func(t *testing.T) {
		saved := f.store.headNode.slot
		f.store.headNode.slot = 97
		driftGenesisTime(f, 98, 0)
		require.Equal(t, childRoot, f.GetProposerHead())
		f.store.headNode.slot = saved
		driftGenesisTime(f, 3, 1*time.Second)
	})
	t.Run("Not single block reorg", func(t *testing.T) {
		saved := f.store.headNode.parent.slot
		f.store.headNode.parent.slot = 0
		require.Equal(t, childRoot, f.GetProposerHead())
		f.store.headNode.parent.slot = saved
	})
	t.Run("parent is nil", func(t *testing.T) {
		saved := f.store.headNode.parent
		f.store.headNode.parent = nil
		require.Equal(t, childRoot, f.GetProposerHead())
		f.store.headNode.parent = saved
	})
	t.Run("parent is weak", func(t *testing.T) {
		saved := f.store.headNode.parent.weight
		f.store.headNode.parent.weight = 0
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode.parent.weight = saved
	})
	t.Run("Head is strong", func(t *testing.T) {
		f.store.headNode.weight = f.store.committeeWeight
		require.Equal(t, childRoot, f.GetProposerHead())
	})
}
