package doublylinkedtree

import (
	"context"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/forkchoice"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestNode_ApplyWeightChanges_PositiveChange(t *testing.T) {
	f := setup(0, 0)
	ctx := context.Background()
	state, blk, err := prepareForkchoiceState(ctx, f, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, f, 2, indexToHash(2), indexToHash(1), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, f, 3, indexToHash(3), indexToHash(2), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))

	// The updated balances of each node is 100
	s := f.store

	s.emptyNodeByRoot[indexToHash(1)].block.balance = 100
	s.emptyNodeByRoot[indexToHash(2)].block.balance = 100
	s.emptyNodeByRoot[indexToHash(3)].block.balance = 100

	assert.NoError(t, s.treeRootNode.applyWeightChanges(ctx))

	assert.Equal(t, uint64(300), s.emptyNodeByRoot[indexToHash(1)].weight)
	assert.Equal(t, uint64(200), s.emptyNodeByRoot[indexToHash(2)].weight)
	assert.Equal(t, uint64(100), s.emptyNodeByRoot[indexToHash(3)].weight)
}

func TestNode_ApplyWeightChanges_NegativeChange(t *testing.T) {
	f := setup(0, 0)
	ctx := context.Background()
	state, blk, err := prepareForkchoiceState(ctx, f, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, f, 2, indexToHash(2), indexToHash(1), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, f, 3, indexToHash(3), indexToHash(2), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))

	// The updated balances of each node is 100
	s := f.store
	s.emptyNodeByRoot[indexToHash(1)].weight = 400
	s.emptyNodeByRoot[indexToHash(2)].weight = 400
	s.emptyNodeByRoot[indexToHash(3)].weight = 400

	s.emptyNodeByRoot[indexToHash(1)].block.balance = 100
	s.emptyNodeByRoot[indexToHash(2)].block.balance = 100
	s.emptyNodeByRoot[indexToHash(3)].block.balance = 100

	assert.NoError(t, s.treeRootNode.applyWeightChanges(ctx))

	assert.Equal(t, uint64(300), s.emptyNodeByRoot[indexToHash(1)].weight)
	assert.Equal(t, uint64(200), s.emptyNodeByRoot[indexToHash(2)].weight)
	assert.Equal(t, uint64(100), s.emptyNodeByRoot[indexToHash(3)].weight)
}

func TestNode_UpdateBestDescendant_ViableChild(t *testing.T) {
	f := setup(1, 1)
	ctx := context.Background()
	// Input child is the best descendant
	state, blk, err := prepareForkchoiceState(ctx, f, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))

	s := f.store
	assert.Equal(t, 1, len(s.treeRootNode.children))
	assert.Equal(t, s.treeRootNode.children[0], s.treeRootNode.bestDescendant)
}

func TestNode_UpdateBestDescendant_HigherWeightChild(t *testing.T) {
	f := setup(1, 1)
	ctx := context.Background()
	// Input child is the best descendant
	state, blk, err := prepareForkchoiceState(ctx, f, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, f, 2, indexToHash(2), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))

	s := f.store
	s.emptyNodeByRoot[indexToHash(1)].weight = 100
	s.emptyNodeByRoot[indexToHash(2)].weight = 200
	assert.NoError(t, s.treeRootNode.updateBestDescendant(ctx, 1, 1, 1))

	assert.Equal(t, 2, len(s.treeRootNode.children))
	assert.Equal(t, s.treeRootNode.children[1], s.treeRootNode.bestDescendant)
}

func TestNode_UpdateBestDescendant_LowerWeightChild(t *testing.T) {
	f := setup(1, 1)
	ctx := context.Background()
	// Input child is the best descendant
	state, blk, err := prepareForkchoiceState(ctx, f, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, f, 2, indexToHash(2), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))

	s := f.store
	s.emptyNodeByRoot[indexToHash(1)].weight = 200
	s.emptyNodeByRoot[indexToHash(2)].weight = 100
	assert.NoError(t, s.treeRootNode.updateBestDescendant(ctx, 1, 1, 1))

	assert.Equal(t, 2, len(s.treeRootNode.children))
	assert.Equal(t, s.treeRootNode.children[0], s.treeRootNode.bestDescendant)
}

func TestNode_ViableForHead(t *testing.T) {
	tests := []struct {
		n              *Node
		justifiedEpoch primitives.Epoch
		want           bool
	}{
		{&Node{block: &BlockNode{}}, 0, true},
		{&Node{block: &BlockNode{}}, 1, false},
		{&Node{block: &BlockNode{finalizedEpoch: 1, justifiedEpoch: 1}}, 1, true},
		{&Node{block: &BlockNode{finalizedEpoch: 1, justifiedEpoch: 1}}, 2, false},
		{&Node{block: &BlockNode{finalizedEpoch: 1, justifiedEpoch: 2}}, 3, false},
		{&Node{block: &BlockNode{finalizedEpoch: 1, justifiedEpoch: 2}}, 4, false},
		{&Node{block: &BlockNode{finalizedEpoch: 1, justifiedEpoch: 3}}, 4, true},
	}
	for _, tc := range tests {
		got := tc.n.viableForHead(tc.justifiedEpoch, 5)
		assert.Equal(t, tc.want, got)
	}
}

func TestNode_LeadsToViableHead(t *testing.T) {
	f := setup(4, 3)
	ctx := context.Background()
	state, blk, err := prepareForkchoiceState(ctx, f, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, f, 2, indexToHash(2), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, f, 3, indexToHash(3), indexToHash(1), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, f, 4, indexToHash(4), indexToHash(2), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, f, 5, indexToHash(5), indexToHash(3), params.BeaconConfig().ZeroHash, 4, 3)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))

	require.Equal(t, true, f.store.treeRootNode.leadsToViableHead(4, 5))
	require.Equal(t, true, f.store.emptyNodeByRoot[indexToHash(5)].leadsToViableHead(4, 5))
	require.Equal(t, false, f.store.emptyNodeByRoot[indexToHash(2)].leadsToViableHead(4, 5))
	require.Equal(t, false, f.store.emptyNodeByRoot[indexToHash(4)].leadsToViableHead(4, 5))
}

func TestNode_SetFullyValidated(t *testing.T) {
	f := setup(1, 1)
	ctx := context.Background()
	storeNodes := make([]*Node, 6)
	storeNodes[0] = f.store.treeRootNode
	// insert blocks in the fork pattern (optimistic status in parenthesis)
	//
	// 0 (false) -- 1 (false) -- 2 (false) -- 3 (true) -- 4 (true)
	//               \
	//                 -- 5 (true)
	//
	state, blk, err := prepareForkchoiceState(ctx, f, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	storeNodes[1] = f.store.emptyNodeByRoot[blk.Root()]
	require.NoError(t, f.SetOptimisticToValid(ctx, params.BeaconConfig().ZeroHash))
	state, blk, err = prepareForkchoiceState(ctx, f, 2, indexToHash(2), indexToHash(1), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	storeNodes[2] = f.store.emptyNodeByRoot[blk.Root()]
	require.NoError(t, f.SetOptimisticToValid(ctx, indexToHash(1)))
	state, blk, err = prepareForkchoiceState(ctx, f, 3, indexToHash(3), indexToHash(2), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	storeNodes[3] = f.store.emptyNodeByRoot[blk.Root()]
	state, blk, err = prepareForkchoiceState(ctx, f, 4, indexToHash(4), indexToHash(3), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	storeNodes[4] = f.store.emptyNodeByRoot[blk.Root()]
	state, blk, err = prepareForkchoiceState(ctx, f, 5, indexToHash(5), indexToHash(1), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	storeNodes[5] = f.store.emptyNodeByRoot[blk.Root()]
	opt, err := f.IsOptimistic(indexToHash(5))
	require.NoError(t, err)
	require.Equal(t, true, opt)

	opt, err = f.IsOptimistic(indexToHash(4))
	require.NoError(t, err)
	require.Equal(t, true, opt)

	require.NoError(t, f.store.emptyNodeByRoot[indexToHash(4)].setNodeAndParentValidated(ctx))

	// block 5 should still be optimistic
	opt, err = f.IsOptimistic(indexToHash(5))
	require.NoError(t, err)
	require.Equal(t, true, opt)

	// block 4 and 3 should now be valid
	opt, err = f.IsOptimistic(indexToHash(4))
	require.NoError(t, err)
	require.Equal(t, false, opt)

	opt, err = f.IsOptimistic(indexToHash(3))
	require.NoError(t, err)
	require.Equal(t, false, opt)

	respNodes := make([]*forkchoice.Node, 0)
	respNodes, err = f.store.treeRootNode.nodeTreeDump(ctx, respNodes)
	require.NoError(t, err)
	require.Equal(t, len(respNodes), f.NodeCount())

	for i, respNode := range respNodes {
		require.Equal(t, storeNodes[i].block.slot, respNode.Slot)
		require.DeepEqual(t, storeNodes[i].block.root[:], respNode.BlockRoot)
		require.Equal(t, storeNodes[i].block.balance, respNode.Balance)
		require.Equal(t, storeNodes[i].weight, respNode.Weight)
		require.Equal(t, storeNodes[i].optimistic, respNode.ExecutionOptimistic)
		require.Equal(t, storeNodes[i].block.justifiedEpoch, respNode.JustifiedEpoch)
		require.Equal(t, storeNodes[i].block.unrealizedJustifiedEpoch, respNode.UnrealizedJustifiedEpoch)
		require.Equal(t, storeNodes[i].block.finalizedEpoch, respNode.FinalizedEpoch)
		require.Equal(t, storeNodes[i].block.unrealizedFinalizedEpoch, respNode.UnrealizedFinalizedEpoch)
		require.Equal(t, storeNodes[i].block.timestamp, respNode.Timestamp)
	}
}

func TestNode_TimeStampsChecks(t *testing.T) {
	f := setup(0, 0)
	ctx := context.Background()

	// early block
	driftGenesisTime(f, 1, 1)
	root := [32]byte{'a'}
	f.justifiedBalances = []uint64{10}
	state, blk, err := prepareForkchoiceState(ctx, f, 1, root, params.BeaconConfig().ZeroHash, [32]byte{'A'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	headRoot, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, root, headRoot)
	early, err := f.store.headNode.block.arrivedEarly(f.store.genesisTime)
	require.NoError(t, err)
	require.Equal(t, true, early)
	late, err := f.store.headNode.block.arrivedAfterOrphanCheck(f.store.genesisTime)
	require.NoError(t, err)
	require.Equal(t, false, late)

	orphanLateBlockFirstThreshold := params.BeaconConfig().SecondsPerSlot / params.BeaconConfig().IntervalsPerSlot
	// late block
	driftGenesisTime(f, 2, orphanLateBlockFirstThreshold+1)
	root = [32]byte{'b'}
	state, blk, err = prepareForkchoiceState(ctx, f, 2, root, [32]byte{'a'}, [32]byte{'B'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, root, headRoot)
	early, err = f.store.headNode.block.arrivedEarly(f.store.genesisTime)
	require.NoError(t, err)
	require.Equal(t, false, early)
	late, err = f.store.headNode.block.arrivedAfterOrphanCheck(f.store.genesisTime)
	require.NoError(t, err)
	require.Equal(t, false, late)

	// very late block
	driftGenesisTime(f, 3, ProcessAttestationsThreshold+1)
	root = [32]byte{'c'}
	state, blk, err = prepareForkchoiceState(ctx, f, 3, root, [32]byte{'b'}, [32]byte{'C'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, root, headRoot)
	early, err = f.store.headNode.block.arrivedEarly(f.store.genesisTime)
	require.NoError(t, err)
	require.Equal(t, false, early)
	late, err = f.store.headNode.block.arrivedAfterOrphanCheck(f.store.genesisTime)
	require.NoError(t, err)
	require.Equal(t, true, late)

	// block from the future
	root = [32]byte{'d'}
	state, blk, err = prepareForkchoiceState(ctx, f, 5, root, [32]byte{'c'}, [32]byte{'D'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, root, headRoot)
	early, err = f.store.headNode.block.arrivedEarly(f.store.genesisTime)
	require.ErrorContains(t, "invalid timestamp", err)
	require.Equal(t, true, early)
	late, err = f.store.headNode.block.arrivedAfterOrphanCheck(f.store.genesisTime)
	require.ErrorContains(t, "invalid timestamp", err)
	require.Equal(t, false, late)
}
