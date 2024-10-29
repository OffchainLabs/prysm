package doublylinkedtree

import (
	"context"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
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
	ctx := context.Background()
	driftGenesisTime(f, 1, 0)
	st, blk, err := prepareForkchoiceState(ctx, f, 1, [32]byte{'a'}, [32]byte{}, [32]byte{'A'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))
	attesters := make([]uint64, f.numActiveValidators-64)
	for i := range attesters {
		attesters[i] = uint64(i + 64)
	}
	f.ProcessAttestation(ctx, attesters, blk.Root(), 0)

	orphanLateBlockFirstThreshold := params.BeaconConfig().SecondsPerSlot / params.BeaconConfig().IntervalsPerSlot
	driftGenesisTime(f, 2, orphanLateBlockFirstThreshold+1)
	st, blk, err = prepareForkchoiceState(ctx, f, 2, [32]byte{'b'}, [32]byte{'a'}, [32]byte{'B'}, 0, 0)
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
		driftGenesisTime(f, 2, orphanLateBlockFirstThreshold+1)
	})
	t.Run("head is from epoch boundary", func(t *testing.T) {
		saved := f.store.headNode.block.slot
		driftGenesisTime(f, params.BeaconConfig().SlotsPerEpoch-1, 0)
		f.store.headNode.block.slot = params.BeaconConfig().SlotsPerEpoch - 1
		require.Equal(t, false, f.ShouldOverrideFCU())
		driftGenesisTime(f, 2, orphanLateBlockFirstThreshold+1)
		f.store.headNode.block.slot = saved
	})
	t.Run("head is early", func(t *testing.T) {
		saved := f.store.headNode.block.timestamp
		f.store.headNode.block.timestamp = saved - 2
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode.block.timestamp = saved
	})
	t.Run("chain not finalizing", func(t *testing.T) {
		saved := f.store.headNode.block.slot
		f.store.headNode.block.slot = 97
		driftGenesisTime(f, 97, orphanLateBlockFirstThreshold+1)
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode.block.slot = saved
		driftGenesisTime(f, 2, orphanLateBlockFirstThreshold+1)
	})
	t.Run("Not single block reorg", func(t *testing.T) {
		saved := f.store.headNode.block.parent.block.slot
		f.store.headNode.block.parent.block.slot = 0
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode.block.parent.block.slot = saved
	})
	t.Run("parent is nil", func(t *testing.T) {
		saved := f.store.headNode.block.parent
		f.store.headNode.block.parent = nil
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode.block.parent = saved
	})
	t.Run("parent is weak early call", func(t *testing.T) {
		saved := f.store.headNode.block.parent.weight
		f.store.headNode.block.parent.weight = 0
		require.Equal(t, true, f.ShouldOverrideFCU())
		f.store.headNode.block.parent.weight = saved
	})
	t.Run("parent is weak late call", func(t *testing.T) {
		saved := f.store.headNode.block.parent.weight
		driftGenesisTime(f, 2, 11)
		f.store.headNode.block.parent.weight = 0
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode.block.parent.weight = saved
		driftGenesisTime(f, 2, orphanLateBlockFirstThreshold+1)
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
	ctx := context.Background()
	driftGenesisTime(f, 1, 0)
	parentRoot := [32]byte{'a'}
	st, blk, err := prepareForkchoiceState(ctx, f, 1, parentRoot, [32]byte{}, [32]byte{'A'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))
	attesters := make([]uint64, f.numActiveValidators-64)
	for i := range attesters {
		attesters[i] = uint64(i + 64)
	}
	f.ProcessAttestation(ctx, attesters, blk.Root(), 0)

	driftGenesisTime(f, 3, 1)
	childRoot := [32]byte{'b'}
	st, blk, err = prepareForkchoiceState(ctx, f, 2, childRoot, [32]byte{'a'}, [32]byte{'B'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))
	headRoot, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, blk.Root(), headRoot)
	orphanLateBlockFirstThreshold := params.BeaconConfig().SecondsPerSlot / params.BeaconConfig().IntervalsPerSlot
	f.store.headNode.block.timestamp -= params.BeaconConfig().SecondsPerSlot - orphanLateBlockFirstThreshold
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
		driftGenesisTime(f, 3, 1)
	})
	t.Run("head is from epoch boundary", func(t *testing.T) {
		saved := f.store.headNode.block.slot
		driftGenesisTime(f, params.BeaconConfig().SlotsPerEpoch, 0)
		f.store.headNode.block.slot = params.BeaconConfig().SlotsPerEpoch - 1
		require.Equal(t, childRoot, f.GetProposerHead())
		driftGenesisTime(f, 3, 1)
		f.store.headNode.block.slot = saved
	})
	t.Run("head is early", func(t *testing.T) {
		saved := f.store.headNode.block.timestamp
		f.store.headNode.block.timestamp = saved - 2
		require.Equal(t, childRoot, f.GetProposerHead())
		f.store.headNode.block.timestamp = saved
	})
	t.Run("chain not finalizing", func(t *testing.T) {
		saved := f.store.headNode.block.slot
		f.store.headNode.block.slot = 97
		driftGenesisTime(f, 98, 0)
		require.Equal(t, childRoot, f.GetProposerHead())
		f.store.headNode.block.slot = saved
		driftGenesisTime(f, 3, 1)
	})
	t.Run("Not single block reorg", func(t *testing.T) {
		saved := f.store.headNode.block.parent.block.slot
		f.store.headNode.block.parent.block.slot = 0
		require.Equal(t, childRoot, f.GetProposerHead())
		f.store.headNode.block.parent.block.slot = saved
	})
	t.Run("parent is nil", func(t *testing.T) {
		saved := f.store.headNode.block.parent
		f.store.headNode.block.parent = nil
		require.Equal(t, childRoot, f.GetProposerHead())
		f.store.headNode.block.parent = saved
	})
	t.Run("parent is weak", func(t *testing.T) {
		saved := f.store.headNode.block.parent.weight
		f.store.headNode.block.parent.weight = 0
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode.block.parent.weight = saved
	})
	t.Run("Head is strong", func(t *testing.T) {
		f.store.headNode.weight = f.store.committeeWeight
		require.Equal(t, childRoot, f.GetProposerHead())
	})
}
