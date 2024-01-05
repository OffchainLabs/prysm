package doublylinkedtree

import (
	"context"
	"strings"
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
	state, blk, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, 2, indexToHash(2), indexToHash(1), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, 3, indexToHash(3), indexToHash(2), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))

	// The updated balances of each node is 100
	s := f.store

	s.nodeByRoot[indexToHash(1)].balance = 100
	s.nodeByRoot[indexToHash(2)].balance = 100
	s.nodeByRoot[indexToHash(3)].balance = 100

	assert.NoError(t, s.treeRootNode.applyWeightChanges(ctx, params.BeaconConfig().ZeroHash, 0))

	assert.Equal(t, uint64(300), s.nodeByRoot[indexToHash(1)].weight)
	assert.Equal(t, uint64(200), s.nodeByRoot[indexToHash(2)].weight)
	assert.Equal(t, uint64(100), s.nodeByRoot[indexToHash(3)].weight)
	assert.Equal(t, uint64(300), s.nodeByRoot[indexToHash(1)].weightWithoutBoost)
	assert.Equal(t, uint64(200), s.nodeByRoot[indexToHash(2)].weightWithoutBoost)
	assert.Equal(t, uint64(100), s.nodeByRoot[indexToHash(3)].weightWithoutBoost)
}

func TestNode_ApplyWeightChanges_NegativeChange(t *testing.T) {
	f := setup(0, 0)
	ctx := context.Background()
	state, blk, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, 2, indexToHash(2), indexToHash(1), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, 3, indexToHash(3), indexToHash(2), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))

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
	assert.Equal(t, uint64(300), s.nodeByRoot[indexToHash(1)].weightWithoutBoost)
	assert.Equal(t, uint64(200), s.nodeByRoot[indexToHash(2)].weightWithoutBoost)
	assert.Equal(t, uint64(100), s.nodeByRoot[indexToHash(3)].weightWithoutBoost)
}

func TestNode_applyWeightChanges(t *testing.T) {
	rootA := [32]byte{0x01}
	rootB := [32]byte{0x02}
	rootC := [32]byte{0x03}

	t.Run("proposer boost applied and subtracted from weightWithoutBoost", func(t *testing.T) {
		nodeC := &Node{
			root:    rootC,
			balance: 20,
		}
		nodeB := &Node{
			root:     rootB,
			balance:  40,
			children: []*Node{nodeC},
		}
		nodeA := &Node{
			root:     rootA,
			balance:  100,
			children: []*Node{nodeB},
		}

		ctx := context.Background()
		err := nodeA.applyWeightChanges(ctx, rootC, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// nodeC has no children: weight = balance = 20
		if nodeC.weight != 20 || nodeC.weightWithoutBoost != 10 {
			t.Errorf("nodeC weight = %d, weightWithoutBoost = %d, want both = 20", nodeC.weight, nodeC.weightWithoutBoost)
		}

		// nodeB has one child (C): weight = 40 + 20 = 60
		// weightWithoutBoost = 40 + 20 - 10 = 50 (because pbRoot == rootC)
		if nodeB.weight != 60 {
			t.Errorf("nodeB weight = %d, want 60", nodeB.weight)
		}
		if nodeB.weightWithoutBoost != 50 {
			t.Errorf("nodeB weightWithoutBoost = %d, want 50", nodeB.weightWithoutBoost)
		}
		// nodeA has one child (B): weight = 100 + 60 = 160
		// weightWithoutBoost = 100 + 60 - 10 = 150 (because pbRoot == rootC)
		if nodeA.weight != 160 {
			t.Errorf("nodeA weight = %d, want 160", nodeA.weight)
		}
		if nodeA.weightWithoutBoost != 150 {
			t.Errorf("nodeA weightWithoutBoost = %d, want 150", nodeA.weightWithoutBoost)
		}
	})

	t.Run("returns error if proposer boost value exceeds node balance", func(t *testing.T) {
		node := &Node{
			root:    rootB,
			balance: 5,
		}
		err := node.applyWeightChanges(context.Background(), rootB, 10)
		if err == nil || !strings.Contains(err.Error(), "less than proposer boost") {
			t.Errorf("expected balance error, got: %v", err)
		}
	})

	t.Run("skips applying boost if root does not match pbRoot", func(t *testing.T) {
		node := &Node{
			root:    rootB,
			balance: 50,
		}
		err := node.applyWeightChanges(context.Background(), rootC, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if node.weight != 50 || node.weightWithoutBoost != 50 {
			t.Errorf("weight = %d, weightWithoutBoost = %d, want both = 50", node.weight, node.weightWithoutBoost)
		}
	})

	t.Run("aborts on context cancel", func(t *testing.T) {
		child := &Node{
			root:    rootC,
			balance: 25,
		}
		node := &Node{
			root:     rootB,
			balance:  10,
			children: []*Node{child},
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := node.applyWeightChanges(ctx, rootC, 5)
		if err != context.Canceled {
			t.Errorf("expected context.Canceled error, got: %v", err)
		}
	})
}

func TestNode_UpdateBestDescendant_NonViableChild(t *testing.T) {
	f := setup(1, 1)
	ctx := context.Background()
	// Input child is not viable.
	state, blk, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 2, 3)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))

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
	state, blk, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
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
	state, blk, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, 2, indexToHash(2), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))

	s := f.store
	s.nodeByRoot[indexToHash(1)].weight = 100
	s.nodeByRoot[indexToHash(2)].weight = 200
	assert.NoError(t, s.treeRootNode.updateBestDescendant(ctx, 1, 1, 2, 0, f.store.committeeWeight))

	assert.Equal(t, 2, len(s.treeRootNode.children))
	assert.Equal(t, s.treeRootNode.children[1], s.treeRootNode.bestDescendant)
}

func TestNode_UpdateBestDescendant_BestConfirmedDescendant(t *testing.T) {
	ctx := context.Background()
	f := setup(1, 1)

	// Insert first child node
	state1, blk1, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state1, blk1))

	// Insert second child node
	state2, blk2, err := prepareForkchoiceState(ctx, 2, indexToHash(2), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state2, blk2))

	s := f.store

	// Set weightWithoutBoost manually to control confirmation logic
	node1 := s.nodeByRoot[indexToHash(1)]
	node2 := s.nodeByRoot[indexToHash(2)]

	node1.weightWithoutBoost = 100
	node2.weightWithoutBoost = 200

	// Execute update
	err = s.treeRootNode.updateBestDescendant(ctx, 1, 1, 3, 0, f.store.committeeWeight)
	require.NoError(t, err)

	// Assert the correct bestConfirmedDescendant is selected
	assert.NotNil(t, s.treeRootNode.bestConfirmedDescendant, "expected bestConfirmedDescendant to be set")
	assert.Equal(t, node2, s.treeRootNode.bestConfirmedDescendant, "expected node2 to be the bestConfirmedDescendant")

	// Additional: verify that the best descendant logic is consistent
	assert.Equal(t, node2, s.treeRootNode.bestDescendant, "expected node2 to be the bestDescendant")
}

func TestNode_UpdateBestDescendant_LowerWeightChild(t *testing.T) {
	f := setup(1, 1)
	ctx := context.Background()
	// Input child is the best descendant
	state, blk, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, 2, indexToHash(2), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))

	s := f.store
	s.nodeByRoot[indexToHash(1)].weight = 200
	s.nodeByRoot[indexToHash(2)].weight = 100
	assert.NoError(t, s.treeRootNode.updateBestDescendant(ctx, 1, 1, 2, 0, f.store.committeeWeight))

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
	state, blk, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, 2, indexToHash(2), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, 3, indexToHash(3), indexToHash(1), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, 4, indexToHash(4), indexToHash(2), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	state, blk, err = prepareForkchoiceState(ctx, 5, indexToHash(5), indexToHash(3), params.BeaconConfig().ZeroHash, 4, 3)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))

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
	state, blk, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	storeNodes[1] = f.store.nodeByRoot[blk.Root()]
	require.NoError(t, f.SetOptimisticToValid(ctx, params.BeaconConfig().ZeroHash))
	state, blk, err = prepareForkchoiceState(ctx, 2, indexToHash(2), indexToHash(1), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	storeNodes[2] = f.store.nodeByRoot[blk.Root()]
	require.NoError(t, f.SetOptimisticToValid(ctx, indexToHash(1)))
	state, blk, err = prepareForkchoiceState(ctx, 3, indexToHash(3), indexToHash(2), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	storeNodes[3] = f.store.nodeByRoot[blk.Root()]
	state, blk, err = prepareForkchoiceState(ctx, 4, indexToHash(4), indexToHash(3), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	storeNodes[4] = f.store.nodeByRoot[blk.Root()]
	state, blk, err = prepareForkchoiceState(ctx, 5, indexToHash(5), indexToHash(1), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	storeNodes[5] = f.store.nodeByRoot[blk.Root()]

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
	state, blk, err := prepareForkchoiceState(ctx, 1, root, params.BeaconConfig().ZeroHash, [32]byte{'A'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
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
	state, blk, err = prepareForkchoiceState(ctx, 2, root, [32]byte{'a'}, [32]byte{'B'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
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
	state, blk, err = prepareForkchoiceState(ctx, 3, root, [32]byte{'b'}, [32]byte{'C'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
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
	state, blk, err = prepareForkchoiceState(ctx, 5, root, [32]byte{'c'}, [32]byte{'D'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
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

func TestNode_maxWeight(t *testing.T) {
	type fields struct {
		slot   primitives.Slot
		parent *Node
	}
	type args struct {
		endSlot         primitives.Slot
		committeeWeight uint64
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   uint64
	}{
		{
			name: "startSlot > endSlot, should return 0",
			fields: fields{
				parent: &Node{slot: 9},
			},
			args: args{
				endSlot: 9,
			},
			want: 0,
		},
		{
			name: "startEpoch == currentEpoch",
			fields: fields{
				parent: &Node{slot: 4},
			},
			args: args{
				endSlot:         7,
				committeeWeight: 10,
			},
			want: 30, // (7 - 5 + 1) = 30
		},
		{
			name: "currentEpoch > startEpoch + 1",
			fields: fields{
				slot: 0,
			},
			args: args{
				endSlot:         32,
				committeeWeight: 10,
			},
			want: 320, // slotsPerEpoch * committeeWeight
		},
		{
			name: "currentEpoch == startEpoch+1 && startSlot % slotsPerEpoch == 0",
			fields: fields{
				parent: &Node{
					slot: 31,
				},
			},
			args: args{
				endSlot:         64,
				committeeWeight: 5,
			},
			want: 160, // slotsPerEpoch * committeeWeight
		},
		{
			name: "partial overlap between epochs",
			fields: fields{
				slot: 30,
			},
			args: args{
				endSlot:         33,
				committeeWeight: 4,
			},
			want: func() uint64 {
				startSlot := uint64(30)
				currentSlot := uint64(33)
				slotsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch)
				slotsInStartEpoch := slotsPerEpoch - (startSlot % slotsPerEpoch) // 32 - 30 = 2
				slotsInCurrentEpoch := (currentSlot % slotsPerEpoch) + 1         // 33 % 32 + 1 = 2

				weightStart := (4 * slotsInStartEpoch * (slotsPerEpoch - slotsInCurrentEpoch)) / slotsPerEpoch // 4 * 2 * 30 / 32 = 7 (int division)
				weightCurrent := 4 * slotsInCurrentEpoch                                                       // 4 * 2 = 8
				return weightStart + weightCurrent                                                             // 7 + 8 = 15
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &Node{
				slot:   tt.fields.slot,
				parent: tt.fields.parent,
			}
			if got := n.maxWeight(tt.args.endSlot, tt.args.committeeWeight); got != tt.want {
				t.Errorf("maxWeight() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNode_confirmed(t *testing.T) {
	type fields struct {
		nodeSlot           primitives.Slot
		weightWithoutBoost uint64
	}
	type args struct {
		slot            primitives.Slot
		committeeWeight uint64
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name: "node slot > slot returns false",
			fields: fields{
				nodeSlot: 10,
			},
			args: args{
				slot: 9,
			},
			want: false,
		},
		{
			name: "weight without boost <= threshold returns false",
			fields: fields{
				weightWithoutBoost: 1620,
			},
			args: args{
				slot:            32,
				committeeWeight: 100,
			},
			want: false, // maxWeight = 3200, pbWeight = 40, threshold = 1620
		},
		{
			name: "weight without boost > threshold returns true",
			fields: fields{
				weightWithoutBoost: 1621,
			},
			args: args{
				slot:            32,
				committeeWeight: 100,
			},
			want: true, // maxWeight = 3200, pbWeight = 40, threshold = 1620
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &Node{
				slot:               tt.fields.nodeSlot,
				weightWithoutBoost: tt.fields.weightWithoutBoost,
			}
			if got := n.confirmed(tt.args.slot, tt.args.committeeWeight); got != tt.want {
				t.Errorf("confirmed() = %v, want %v", got, tt.want)
			}
		})
	}
}
