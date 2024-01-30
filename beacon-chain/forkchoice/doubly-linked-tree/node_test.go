package doublylinkedtree

import (
	"context"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/forkchoice"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

func TestNode_ApplyWeightChanges_PositiveChange(t *testing.T) {
	f := setup(0, 0)
	ctx := context.Background()
	state, blkRoot, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 2, indexToHash(2), indexToHash(1), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 3, indexToHash(3), indexToHash(2), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))

	// The updated balances of each node is 100
	s := f.store

	s.nodeByRoot[indexToHash(1)].balance = 100
	s.nodeByRoot[indexToHash(2)].balance = 100
	s.nodeByRoot[indexToHash(3)].balance = 100

	assert.NoError(t, s.treeRootNode.applyWeightChanges(ctx, params.BeaconConfig().ZeroHash, 0))

	assert.Equal(t, uint64(300), s.nodeByRoot[indexToHash(1)].weight)
	assert.Equal(t, uint64(200), s.nodeByRoot[indexToHash(2)].weight)
	assert.Equal(t, uint64(100), s.nodeByRoot[indexToHash(3)].weight)
}

func TestNode_ApplyWeightChanges_NegativeChange(t *testing.T) {
	f := setup(0, 0)
	ctx := context.Background()
	state, blkRoot, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 2, indexToHash(2), indexToHash(1), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 3, indexToHash(3), indexToHash(2), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))

	// The updated balances of each node is 100
	s := f.store
	s.nodeByRoot[indexToHash(1)].weight = 400
	s.nodeByRoot[indexToHash(2)].weight = 400
	s.nodeByRoot[indexToHash(3)].weight = 400

	s.nodeByRoot[indexToHash(1)].balance = 100
	s.nodeByRoot[indexToHash(2)].balance = 100
	s.nodeByRoot[indexToHash(3)].balance = 100

	assert.NoError(t, s.treeRootNode.applyWeightChanges(ctx, params.BeaconConfig().ZeroHash, 0))

	assert.Equal(t, uint64(300), s.nodeByRoot[indexToHash(1)].weight)
	assert.Equal(t, uint64(200), s.nodeByRoot[indexToHash(2)].weight)
	assert.Equal(t, uint64(100), s.nodeByRoot[indexToHash(3)].weight)
}

func TestNode_UpdateBestDescendant_NonViableChild(t *testing.T) {
	f := setup(1, 1)
	ctx := context.Background()
	// Input child is not viable.
	state, blkRoot, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 2, 3)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))

	// Verify parent's best child and best descendant are `none`.
	s := f.store
	assert.Equal(t, 1, len(s.treeRootNode.children))
	nilBestDescendant := s.treeRootNode.bestDescendant == nil
	assert.Equal(t, true, nilBestDescendant)
}

func TestNode_UpdateBestDescendant_ViableChild(t *testing.T) {
	f := setup(1, 1)
	ctx := context.Background()
	// Input child is the best descendant
	state, blkRoot, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))

	s := f.store
	assert.Equal(t, 1, len(s.treeRootNode.children))
	assert.Equal(t, s.treeRootNode.children[0], s.treeRootNode.bestDescendant)
}

func TestNode_UpdateBestDescendant_HigherWeightChild(t *testing.T) {
	f := setup(1, 1)
	ctx := context.Background()
	// Input child is the best descendant
	state, blkRoot, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 2, indexToHash(2), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))

	s := f.store
	s.nodeByRoot[indexToHash(1)].weight = 100
	s.nodeByRoot[indexToHash(2)].weight = 200
	assert.NoError(t, s.treeRootNode.updateBestDescendant(ctx, 1, 1, 2, f.store.committeeWeight))

	assert.Equal(t, 2, len(s.treeRootNode.children))
	assert.Equal(t, s.treeRootNode.children[1], s.treeRootNode.bestDescendant)
}

func TestNode_UpdateBestDescendant_LowerWeightChild(t *testing.T) {
	f := setup(1, 1)
	ctx := context.Background()
	// Input child is the best descendant
	state, blkRoot, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 2, indexToHash(2), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))

	s := f.store
	s.nodeByRoot[indexToHash(1)].weight = 200
	s.nodeByRoot[indexToHash(2)].weight = 100
	assert.NoError(t, s.treeRootNode.updateBestDescendant(ctx, 1, 1, 2, f.store.committeeWeight))

	assert.Equal(t, 2, len(s.treeRootNode.children))
	assert.Equal(t, s.treeRootNode.children[0], s.treeRootNode.bestDescendant)
}

func TestNode_ViableForHead(t *testing.T) {
	tests := []struct {
		n              *Node
		justifiedEpoch primitives.Epoch
		want           bool
	}{
		{&Node{}, 0, true},
		{&Node{}, 1, false},
		{&Node{finalizedEpoch: 1, justifiedEpoch: 1}, 1, true},
		{&Node{finalizedEpoch: 1, justifiedEpoch: 1}, 2, false},
		{&Node{finalizedEpoch: 1, justifiedEpoch: 2}, 3, false},
		{&Node{finalizedEpoch: 1, justifiedEpoch: 2}, 4, false},
		{&Node{finalizedEpoch: 1, justifiedEpoch: 3}, 4, true},
	}
	for _, tc := range tests {
		got := tc.n.viableForHead(tc.justifiedEpoch, 5)
		assert.Equal(t, tc.want, got)
	}
}

func TestNode_LeadsToViableHead(t *testing.T) {
	f := setup(4, 3)
	ctx := context.Background()
	state, blkRoot, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 2, indexToHash(2), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 3, indexToHash(3), indexToHash(1), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 4, indexToHash(4), indexToHash(2), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 5, indexToHash(5), indexToHash(3), params.BeaconConfig().ZeroHash, 4, 3)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))

	require.Equal(t, true, f.store.treeRootNode.leadsToViableHead(4, 5))
	require.Equal(t, true, f.store.nodeByRoot[indexToHash(5)].leadsToViableHead(4, 5))
	require.Equal(t, false, f.store.nodeByRoot[indexToHash(2)].leadsToViableHead(4, 5))
	require.Equal(t, false, f.store.nodeByRoot[indexToHash(4)].leadsToViableHead(4, 5))
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
	state, blkRoot, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	storeNodes[1] = f.store.nodeByRoot[blkRoot]
	require.NoError(t, f.SetOptimisticToValid(ctx, params.BeaconConfig().ZeroHash))
	state, blkRoot, err = prepareForkchoiceState(ctx, 2, indexToHash(2), indexToHash(1), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	storeNodes[2] = f.store.nodeByRoot[blkRoot]
	require.NoError(t, f.SetOptimisticToValid(ctx, indexToHash(1)))
	state, blkRoot, err = prepareForkchoiceState(ctx, 3, indexToHash(3), indexToHash(2), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	storeNodes[3] = f.store.nodeByRoot[blkRoot]
	state, blkRoot, err = prepareForkchoiceState(ctx, 4, indexToHash(4), indexToHash(3), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	storeNodes[4] = f.store.nodeByRoot[blkRoot]
	state, blkRoot, err = prepareForkchoiceState(ctx, 5, indexToHash(5), indexToHash(1), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	storeNodes[5] = f.store.nodeByRoot[blkRoot]

	opt, err := f.IsOptimistic(indexToHash(5))
	require.NoError(t, err)
	require.Equal(t, true, opt)

	opt, err = f.IsOptimistic(indexToHash(4))
	require.NoError(t, err)
	require.Equal(t, true, opt)

	require.NoError(t, f.store.nodeByRoot[indexToHash(4)].setNodeAndParentValidated(ctx))

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
		require.Equal(t, storeNodes[i].slot, respNode.Slot)
		require.DeepEqual(t, storeNodes[i].root[:], respNode.BlockRoot)
		require.Equal(t, storeNodes[i].balance, respNode.Balance)
		require.Equal(t, storeNodes[i].weight, respNode.Weight)
		require.Equal(t, storeNodes[i].optimistic, respNode.ExecutionOptimistic)
		require.Equal(t, storeNodes[i].justifiedEpoch, respNode.JustifiedEpoch)
		require.Equal(t, storeNodes[i].unrealizedJustifiedEpoch, respNode.UnrealizedJustifiedEpoch)
		require.Equal(t, storeNodes[i].finalizedEpoch, respNode.FinalizedEpoch)
		require.Equal(t, storeNodes[i].unrealizedFinalizedEpoch, respNode.UnrealizedFinalizedEpoch)
		require.Equal(t, storeNodes[i].timestamp, respNode.Timestamp)
	}
}

func TestNode_TimeStampsChecks(t *testing.T) {
	f := setup(0, 0)
	ctx := context.Background()

	// early block
	driftGenesisTime(f, 1, 1)
	root := [32]byte{'a'}
	f.justifiedBalances = []uint64{10}
	state, blkRoot, err := prepareForkchoiceState(ctx, 1, root, params.BeaconConfig().ZeroHash, [32]byte{'A'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	headRoot, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, root, headRoot)
	early, err := f.store.headNode.arrivedEarly(f.store.genesisTime)
	require.NoError(t, err)
	require.Equal(t, true, early)
	late, err := f.store.headNode.arrivedAfterOrphanCheck(f.store.genesisTime)
	require.NoError(t, err)
	require.Equal(t, false, late)

	orphanLateBlockFirstThreshold := params.BeaconConfig().SecondsPerSlot / params.BeaconConfig().IntervalsPerSlot
	// late block
	driftGenesisTime(f, 2, orphanLateBlockFirstThreshold+1)
	root = [32]byte{'b'}
	state, blkRoot, err = prepareForkchoiceState(ctx, 2, root, [32]byte{'a'}, [32]byte{'B'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, root, headRoot)
	early, err = f.store.headNode.arrivedEarly(f.store.genesisTime)
	require.NoError(t, err)
	require.Equal(t, false, early)
	late, err = f.store.headNode.arrivedAfterOrphanCheck(f.store.genesisTime)
	require.NoError(t, err)
	require.Equal(t, false, late)

	// very late block
	driftGenesisTime(f, 3, ProcessAttestationsThreshold+1)
	root = [32]byte{'c'}
	state, blkRoot, err = prepareForkchoiceState(ctx, 3, root, [32]byte{'b'}, [32]byte{'C'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, root, headRoot)
	early, err = f.store.headNode.arrivedEarly(f.store.genesisTime)
	require.NoError(t, err)
	require.Equal(t, false, early)
	late, err = f.store.headNode.arrivedAfterOrphanCheck(f.store.genesisTime)
	require.NoError(t, err)
	require.Equal(t, true, late)

	// block from the future
	root = [32]byte{'d'}
	state, blkRoot, err = prepareForkchoiceState(ctx, 5, root, [32]byte{'c'}, [32]byte{'D'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, root, headRoot)
	early, err = f.store.headNode.arrivedEarly(f.store.genesisTime)
	require.ErrorContains(t, "invalid timestamp", err)
	require.Equal(t, true, early)
	late, err = f.store.headNode.arrivedAfterOrphanCheck(f.store.genesisTime)
	require.ErrorContains(t, "invalid timestamp", err)
	require.Equal(t, false, late)
}

func TestNode_MaxPossibleSupport(t *testing.T) {
	f := setup(0, 0)
	ctx := context.Background()

	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	require.Equal(t, primitives.Slot(32), slotsPerEpoch)

	state, blkRoot, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	for i := 2; i < 64; i++ {
		state, blkRoot, err = prepareForkchoiceState(ctx, primitives.Slot(i), indexToHash(uint64(i)), indexToHash(uint64(i-1)), params.BeaconConfig().ZeroHash, 0, 0)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	}

	tests := []struct {
		name            string
		nodeIndex       uint64
		currentSlot     primitives.Slot
		committeeWeight uint64
		wantValue       uint64
	}{
		{
			name:            "2 slots in same epoch",
			nodeIndex:       32,
			currentSlot:     33,
			committeeWeight: 1,
			wantValue:       2,
		},
		{
			name:            "31 slots in same epoch",
			nodeIndex:       32,
			currentSlot:     63,
			committeeWeight: 1,
			wantValue:       32,
		},
		{
			name:            "one full epoch",
			nodeIndex:       32,
			currentSlot:     64,
			committeeWeight: 100,
			wantValue:       3200,
		},
		{
			name:            "more than one full epoch",
			nodeIndex:       32,
			currentSlot:     100,
			committeeWeight: 100,
			wantValue:       3200,
		},
		{
			name:            "epoch mid to epoch mid",
			nodeIndex:       16,
			currentSlot:     48,
			committeeWeight: 100,
			wantValue:       2450,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			driftGenesisTime(f, tc.currentSlot, 0)
			require.Equal(t, tc.currentSlot, slots.CurrentSlot(f.store.genesisTime))

			s := f.store
			node := s.nodeByRoot[indexToHash(tc.nodeIndex)]
			mps := node.getMaxPossibleSupport(tc.currentSlot, tc.committeeWeight)
			require.Equal(t, tc.wantValue, mps)
		})
	}
}
