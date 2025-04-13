package doublylinkedtree

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/forkchoice"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestNode_ApplyWeightChanges_PositiveChange(t *testing.T) {
	f := setup(0, 0)
	ctx := t.Context()
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

	assert.NoError(t, s.treeRootNode.applyWeightChanges(ctx))

	assert.Equal(t, uint64(300), s.nodeByRoot[indexToHash(1)].weight)
	assert.Equal(t, uint64(200), s.nodeByRoot[indexToHash(2)].weight)
	assert.Equal(t, uint64(100), s.nodeByRoot[indexToHash(3)].weight)
}

func TestNode_ApplyWeightChanges_NegativeChange(t *testing.T) {
	f := setup(0, 0)
	ctx := t.Context()
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

	assert.NoError(t, s.treeRootNode.applyWeightChanges(ctx))

	assert.Equal(t, uint64(300), s.nodeByRoot[indexToHash(1)].weight)
	assert.Equal(t, uint64(200), s.nodeByRoot[indexToHash(2)].weight)
	assert.Equal(t, uint64(100), s.nodeByRoot[indexToHash(3)].weight)
}

func TestNode_UpdateBestDescendant_NonViableChild(t *testing.T) {
	f := setup(1, 1)
	ctx := t.Context()
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
	ctx := t.Context()
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
	ctx := t.Context()
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

	assert.NoError(t, s.treeRootNode.updateBestDescendant(ctx, &updateDescendantArgs{
		justifiedEpoch:        1,
		finalizedEpoch:        1,
		currentSlot:           2,
		secondsSinceSlotStart: 0,
		committeeWeight:       f.store.committeeWeight,
	}))

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

	node1.weight = 100
	node2.weight = 200

	// Execute update
	assert.NoError(t, s.treeRootNode.updateBestDescendant(ctx, &updateDescendantArgs{
		justifiedEpoch:        1,
		finalizedEpoch:        1,
		currentSlot:           3,
		secondsSinceSlotStart: 0,
		committeeWeight:       f.store.committeeWeight,
	}))
	require.NoError(t, err)

	// Assert the correct bestConfirmedDescendant is selected
	assert.NotNil(t, s.treeRootNode.bestConfirmedDescendant, "expected bestConfirmedDescendant to be set")
	assert.Equal(t, node2, s.treeRootNode.bestConfirmedDescendant, "expected node2 to be the bestConfirmedDescendant")

	// Additional: verify that the best descendant logic is consistent
	assert.Equal(t, node2, s.treeRootNode.bestDescendant, "expected node2 to be the bestDescendant")
}

func TestNode_UpdateBestDescendant_LowerWeightChild(t *testing.T) {
	f := setup(1, 1)
	ctx := t.Context()
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
	assert.NoError(t, s.treeRootNode.updateBestDescendant(ctx, &updateDescendantArgs{
		justifiedEpoch:        1,
		finalizedEpoch:        1,
		currentSlot:           2,
		secondsSinceSlotStart: 0,
		committeeWeight:       f.store.committeeWeight,
	}))

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
	ctx := t.Context()
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
	ctx := t.Context()
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
	ctx := t.Context()

	// early block
	driftGenesisTime(f, 1, time.Second)
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

	orphanLateBlockFirstThreshold := time.Duration(params.BeaconConfig().SecondsPerSlot/params.BeaconConfig().IntervalsPerSlot) * time.Second
	// late block
	driftGenesisTime(f, 2, orphanLateBlockFirstThreshold+time.Second)
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
	driftGenesisTime(f, 3, ProcessAttestationsThreshold+time.Second)
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
	cfg := params.BeaconConfig()
	cfg.FastConfirmationByzantineThreshold = 33
	undo, err := params.SetActiveWithUndo(cfg)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, undo())
	}()

	type fields struct {
		nodeSlot       primitives.Slot
		weight         uint64
		root           [32]byte
		bestDescendant *Node
	}
	type args struct {
		slot            primitives.Slot
		committeeWeight uint64
		pbRoot          [32]byte
		pbValue         uint64
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
				weight:         186, // 200 committee weight, 40 pb weight, 66 byzantine weight
				bestDescendant: &Node{},
			},
			args: args{
				slot:            1,
				committeeWeight: 100,
			},
			want: false,
		},
		{
			name: "weight without boost > threshold returns true",
			fields: fields{
				weight:         187, // 200 committee weight, 40 pb weight, 66 byzantine weight
				bestDescendant: &Node{},
			},
			args: args{
				slot:            1,
				committeeWeight: 100,
			},
			want: true,
		},
		{
			name: "node root matches pbRoot but balance < pbValue returns false",
			fields: fields{
				weight: 187, bestDescendant: &Node{
					root: [32]byte{1},
				},
			},
			args: args{
				slot:            1,
				committeeWeight: 100,
				pbRoot:          [32]byte{1},
				pbValue:         100000000,
			},
			want: false,
		},
		{
			name: "node root matches pbRoot, balance >= pbValue, adjusted weight <= threshold returns false",
			fields: fields{
				weight: 187,
				bestDescendant: &Node{
					root: [32]byte{1},
				},
			},
			args: args{
				slot:            1,
				committeeWeight: 100,
				pbRoot:          [32]byte{1},
				pbValue:         1,
			},
			want: false,
		},
		{
			name: "node root matches pbRoot (self), balance >= pbValue, adjusted weight <= threshold returns false",
			fields: fields{
				weight: 187,
				root:   [32]byte{1},
			},
			args: args{
				slot:            1,
				committeeWeight: 100,
				pbRoot:          [32]byte{1},
				pbValue:         1,
			},
			want: false,
		},
		{
			name: "node root matches pbRoot, balance >= pbValue, adjusted weight > threshold returns true",
			fields: fields{
				weight: 188,
				bestDescendant: &Node{
					root: [32]byte{1},
				},
			},
			args: args{
				slot:            1,
				committeeWeight: 100,
				pbRoot:          [32]byte{1},
				pbValue:         1,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &Node{
				slot:           tt.fields.nodeSlot,
				weight:         tt.fields.weight,
				root:           tt.fields.root,
				bestDescendant: tt.fields.bestDescendant,
			}
			if got := n.confirmed(tt.args.slot, tt.args.committeeWeight, tt.args.pbRoot, tt.args.pbValue); got != tt.want {
				t.Errorf("confirmed() = %v, want %v", got, tt.want)
			}
		})
	}
}
