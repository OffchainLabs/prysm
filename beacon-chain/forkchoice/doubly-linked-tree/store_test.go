package doublylinkedtree

import (
	"context"
	"testing"
	"time"

	forkchoicetypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/types"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestStore_JustifiedEpoch(t *testing.T) {
	j := primitives.Epoch(100)
	f := setup(j, j)
	require.Equal(t, j, f.JustifiedCheckpoint().Epoch)
}

func TestStore_FinalizedEpoch(t *testing.T) {
	j := primitives.Epoch(50)
	f := setup(j, j)
	require.Equal(t, j, f.FinalizedCheckpoint().Epoch)
}

func TestStore_NodeCount(t *testing.T) {
	f := setup(0, 0)
	state, blk, err := prepareForkchoiceState(context.Background(), 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(context.Background(), state, blk))
	require.Equal(t, 2, f.NodeCount())
}

func TestStore_NodeByRoot(t *testing.T) {
	f := setup(0, 0)
	ctx := context.Background()
	state, blkRoot, err := prepareForkchoiceState(context.Background(), 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(context.Background(), 2, indexToHash(2), indexToHash(1), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	node0 := f.store.treeRootNode
	node1 := node0.children[0]
	node2 := node1.children[0]

	expectedRoots := map[[32]byte]*Node{
		params.BeaconConfig().ZeroHash: node0,
		indexToHash(1):                 node1,
		indexToHash(2):                 node2,
	}

	require.Equal(t, 3, f.NodeCount())
	for root, node := range f.store.nodeByRoot {
		v, ok := expectedRoots[root]
		require.Equal(t, ok, true)
		require.Equal(t, v, node)
	}
}

func TestForkChoice_HasNode(t *testing.T) {
	f := setup(0, 0)
	ctx := context.Background()
	state, blkRoot, err := prepareForkchoiceState(context.Background(), 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	require.Equal(t, true, f.HasNode(indexToHash(1)))
}

func TestStore_Head_UnknownJustifiedRoot(t *testing.T) {
	f := setup(0, 0)

	f.store.justifiedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: 1, Root: [32]byte{'a'}}
	_, err := f.store.head(context.Background())
	assert.ErrorContains(t, errUnknownJustifiedRoot.Error(), err)
}

func TestStore_Head_Itself(t *testing.T) {
	f := setup(0, 0)
	state, blkRoot, err := prepareForkchoiceState(context.Background(), 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(context.Background(), state, blkRoot))

	// Since the justified node does not have a best descendant, the best node
	// is itself.
	f.store.justifiedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: 0, Root: indexToHash(1)}
	h, err := f.store.head(context.Background())
	require.NoError(t, err)
	assert.Equal(t, indexToHash(1), h)
}

func TestStore_Head_BestDescendant(t *testing.T) {
	f := setup(0, 0)
	ctx := context.Background()
	state, blkRoot, err := prepareForkchoiceState(context.Background(), 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(context.Background(), 2, indexToHash(2), indexToHash(1), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(context.Background(), 3, indexToHash(3), indexToHash(1), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(context.Background(), 4, indexToHash(4), indexToHash(2), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	f.store.justifiedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: 0, Root: indexToHash(1)}
	h, err := f.store.head(context.Background())
	require.NoError(t, err)
	require.Equal(t, h, indexToHash(4))
}

func TestStore_UpdateBestDescendant_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := setup(0, 0)
	state, blkRoot, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	cancel()
	state, blkRoot, err = prepareForkchoiceState(ctx, 2, indexToHash(2), indexToHash(1), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	err = f.InsertNode(ctx, state, blkRoot)
	require.ErrorContains(t, "context canceled", err)
}

func TestStore_Insert(t *testing.T) {
	// The new node does not have a parent.
	treeRootNode := &Node{slot: 0, root: indexToHash(0)}
	nodeByRoot := map[[32]byte]*Node{indexToHash(0): treeRootNode}
	nodeByPayload := map[[32]byte]*Node{indexToHash(0): treeRootNode}
	jc := &forkchoicetypes.Checkpoint{Epoch: 0}
	fc := &forkchoicetypes.Checkpoint{Epoch: 0}
	s := &Store{nodeByRoot: nodeByRoot, treeRootNode: treeRootNode, nodeByPayload: nodeByPayload, justifiedCheckpoint: jc, finalizedCheckpoint: fc, highestReceivedNode: &Node{}}
	payloadHash := [32]byte{'a'}
	ctx := context.Background()
	_, blk, err := prepareForkchoiceState(ctx, 100, indexToHash(100), indexToHash(0), payloadHash, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, len(s.nodeByRoot), "Did not insert block")
	assert.Equal(t, (*Node)(nil), treeRootNode.parent, "Incorrect parent")
	assert.Equal(t, 1, len(treeRootNode.children), "Incorrect children number")
	assert.Equal(t, payloadHash, treeRootNode.children[0].payloadHash, "Incorrect payload hash")
	child := treeRootNode.children[0]
	assert.Equal(t, primitives.Epoch(1), child.justifiedEpoch, "Incorrect justification")
	assert.Equal(t, primitives.Epoch(1), child.finalizedEpoch, "Incorrect finalization")
	assert.Equal(t, indexToHash(100), child.root, "Incorrect root")
}

func TestStore_Prune_MoreThanThreshold(t *testing.T) {
	// Define 100 nodes in store.
	numOfNodes := uint64(100)
	f := setup(0, 0)
	ctx := context.Background()
	state, blkRoot, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	for i := uint64(2); i < numOfNodes; i++ {
		state, blkRoot, err = prepareForkchoiceState(ctx, primitives.Slot(i), indexToHash(i), indexToHash(i-1), params.BeaconConfig().ZeroHash, 0, 0)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	}

	s := f.store

	// Finalized root is at index 99 so everything before 99 should be pruned.
	s.finalizedCheckpoint.Root = indexToHash(99)
	require.NoError(t, s.prune(context.Background()))
	assert.Equal(t, 1, len(s.nodeByRoot), "Incorrect nodes count")
}

func TestStore_Prune_MoreThanOnce(t *testing.T) {
	// Define 100 nodes in store.
	numOfNodes := uint64(100)
	f := setup(0, 0)
	ctx := context.Background()
	state, blkRoot, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	for i := uint64(2); i < numOfNodes; i++ {
		state, blkRoot, err = prepareForkchoiceState(ctx, primitives.Slot(i), indexToHash(i), indexToHash(i-1), params.BeaconConfig().ZeroHash, 0, 0)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	}

	s := f.store

	// Finalized root is at index 11 so everything before 11 should be pruned.
	s.finalizedCheckpoint.Root = indexToHash(10)
	require.NoError(t, s.prune(context.Background()))
	assert.Equal(t, 90, len(s.nodeByRoot), "Incorrect nodes count")

	// One more time.
	s.finalizedCheckpoint.Root = indexToHash(20)
	require.NoError(t, s.prune(context.Background()))
	assert.Equal(t, 80, len(s.nodeByRoot), "Incorrect nodes count")
}

func TestStore_Prune_ReturnEarly(t *testing.T) {
	// Define 100 nodes in store.
	numOfNodes := uint64(100)
	f := setup(0, 0)
	ctx := context.Background()
	state, blkRoot, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	for i := uint64(2); i < numOfNodes; i++ {
		state, blkRoot, err = prepareForkchoiceState(ctx, primitives.Slot(i), indexToHash(i), indexToHash(i-1), params.BeaconConfig().ZeroHash, 0, 0)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	}
	require.NoError(t, f.store.prune(ctx))
	nodeCount := f.NodeCount()
	require.NoError(t, f.store.prune(ctx))
	require.Equal(t, nodeCount, f.NodeCount())
}

// This unit test starts with a simple branch like this
//
//   - 1
//     /
//
// -- 0 -- 2
//
// And we finalize 1. As a result only 1 should survive
func TestStore_Prune_NoDanglingBranch(t *testing.T) {
	f := setup(0, 0)
	ctx := context.Background()
	state, blkRoot, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, [32]byte{'1'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 2, indexToHash(2), params.BeaconConfig().ZeroHash, [32]byte{'2'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))

	s := f.store
	s.finalizedCheckpoint.Root = indexToHash(1)
	require.NoError(t, s.prune(context.Background()))
	require.Equal(t, len(s.nodeByRoot), 1)
	require.Equal(t, len(s.nodeByPayload), 1)
}

// This test starts with the following branching diagram
// / We start with the following diagram
//
//	              E -- F
//	             /
//	       C -- D
//	      /      \
//	A -- B        G -- H -- I
//	      \        \
//	       J        -- K -- L
func TestStore_tips(t *testing.T) {
	ctx := context.Background()
	f := setup(1, 1)

	state, blkRoot, err := prepareForkchoiceState(ctx, 100, [32]byte{'a'}, params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 101, [32]byte{'b'}, [32]byte{'a'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 102, [32]byte{'c'}, [32]byte{'b'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 102, [32]byte{'j'}, [32]byte{'b'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 103, [32]byte{'d'}, [32]byte{'c'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 104, [32]byte{'e'}, [32]byte{'d'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 104, [32]byte{'g'}, [32]byte{'d'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 105, [32]byte{'f'}, [32]byte{'e'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 105, [32]byte{'h'}, [32]byte{'g'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 105, [32]byte{'k'}, [32]byte{'g'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 106, [32]byte{'i'}, [32]byte{'h'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 106, [32]byte{'l'}, [32]byte{'k'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	expectedMap := map[[32]byte]primitives.Slot{
		{'f'}: 105,
		{'i'}: 106,
		{'l'}: 106,
		{'j'}: 102,
	}
	roots, slots := f.store.tips()
	for i, r := range roots {
		expectedSlot, ok := expectedMap[r]
		require.Equal(t, true, ok)
		require.Equal(t, slots[i], expectedSlot)
	}
}

func TestStore_PruneMapsNodes(t *testing.T) {
	f := setup(0, 0)
	ctx := context.Background()
	state, blkRoot, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, [32]byte{'1'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 2, indexToHash(2), params.BeaconConfig().ZeroHash, [32]byte{'2'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))

	s := f.store
	s.finalizedCheckpoint.Root = indexToHash(1)
	require.NoError(t, s.prune(context.Background()))
	require.Equal(t, len(s.nodeByRoot), 1)
	require.Equal(t, len(s.nodeByPayload), 1)

}

func TestForkChoice_ReceivedBlocksLastEpoch(t *testing.T) {
	f := setup(1, 1)
	s := f.store
	var b [32]byte

	// Make sure it doesn't underflow
	s.genesisTime = uint64(time.Now().Add(time.Duration(-1*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second).Unix())
	ctx := context.Background()
	_, blk, err := prepareForkchoiceState(ctx, 1, [32]byte{'a'}, b, b, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	count, err := f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)
	slot, _ := f.HighestReceivedBlockSlotRoot()
	require.Equal(t, primitives.Slot(1), slot)
	require.Equal(t, primitives.Slot(0), f.HighestReceivedBlockDelay())

	// 64
	// Received block last epoch is 1
	_, blk, err = prepareForkchoiceState(ctx, 64, [32]byte{'A'}, b, b, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	s.genesisTime = uint64(time.Now().Add(time.Duration((-64*int64(params.BeaconConfig().SecondsPerSlot))-1) * time.Second).Unix())
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)
	slot, _ = f.HighestReceivedBlockSlotRoot()
	require.Equal(t, primitives.Slot(64), slot)
	require.Equal(t, primitives.Slot(0), f.HighestReceivedBlockDelay())

	// 64 65
	// Received block last epoch is 2
	_, blk, err = prepareForkchoiceState(ctx, 65, [32]byte{'B'}, b, b, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	s.genesisTime = uint64(time.Now().Add(time.Duration(-66*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second).Unix())
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(2), count)
	slot, _ = f.HighestReceivedBlockSlotRoot()
	require.Equal(t, primitives.Slot(65), slot)
	require.Equal(t, primitives.Slot(1), f.HighestReceivedBlockDelay())

	// 64 65 66
	// Received block last epoch is 3
	_, blk, err = prepareForkchoiceState(ctx, 66, [32]byte{'C'}, b, b, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	s.genesisTime = uint64(time.Now().Add(time.Duration(-66*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second).Unix())
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(3), count)
	slot, _ = f.HighestReceivedBlockSlotRoot()
	require.Equal(t, primitives.Slot(66), slot)

	// 64 65 66
	//       98
	// Received block last epoch is 1
	_, blk, err = prepareForkchoiceState(ctx, 98, [32]byte{'D'}, b, b, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	s.genesisTime = uint64(time.Now().Add(time.Duration(-98*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second).Unix())
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)
	slot, _ = f.HighestReceivedBlockSlotRoot()
	require.Equal(t, primitives.Slot(98), slot)

	// 64 65 66
	//       98
	//              132
	// Received block last epoch is 1
	_, blk, err = prepareForkchoiceState(ctx, 132, [32]byte{'E'}, b, b, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	s.genesisTime = uint64(time.Now().Add(time.Duration(-132*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second).Unix())
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)
	slot, _ = f.HighestReceivedBlockSlotRoot()
	require.Equal(t, primitives.Slot(132), slot)

	// 64 65 66
	//       98
	//              132
	//       99
	// Received block last epoch is still 1. 99 is outside the window
	_, blk, err = prepareForkchoiceState(ctx, 99, [32]byte{'F'}, b, b, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	s.genesisTime = uint64(time.Now().Add(time.Duration(-132*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second).Unix())
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)
	slot, _ = f.HighestReceivedBlockSlotRoot()
	require.Equal(t, primitives.Slot(132), slot)

	// 64 65 66
	//       98
	//              132
	//       99 100
	// Received block last epoch is still 1. 100 is at the same position as 132
	_, blk, err = prepareForkchoiceState(ctx, 100, [32]byte{'G'}, b, b, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	s.genesisTime = uint64(time.Now().Add(time.Duration(-132*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second).Unix())
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)
	slot, _ = f.HighestReceivedBlockSlotRoot()
	require.Equal(t, primitives.Slot(132), slot)

	// 64 65 66
	//       98
	//              132
	//       99 100 101
	// Received block last epoch is 2. 101 is within the window
	_, blk, err = prepareForkchoiceState(ctx, 101, [32]byte{'H'}, b, b, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	s.genesisTime = uint64(time.Now().Add(time.Duration(-132*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second).Unix())
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(2), count)
	slot, _ = f.HighestReceivedBlockSlotRoot()
	require.Equal(t, primitives.Slot(132), slot)

	s.genesisTime = uint64(time.Now().Add(time.Duration(-134*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second).Unix())
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)
	s.genesisTime = uint64(time.Now().Add(time.Duration(-165*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second).Unix())
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(0), count)
}

func TestStore_TargetRootForEpoch(t *testing.T) {
	ctx := context.Background()
	f := setup(1, 1)

	state, blk, err := prepareForkchoiceState(ctx, params.BeaconConfig().SlotsPerEpoch, [32]byte{'a'}, params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	target, err := f.TargetRootForEpoch(blk.Root(), 1)
	require.NoError(t, err)
	require.Equal(t, target, blk.Root())

	state, blk1, err := prepareForkchoiceState(ctx, params.BeaconConfig().SlotsPerEpoch+1, [32]byte{'b'}, blk.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk1))
	target, err = f.TargetRootForEpoch(blk1.Root(), 1)
	require.NoError(t, err)
	require.Equal(t, target, blk.Root())

	// Insert a block for the next epoch (missed slot 0)

	state, blk2, err := prepareForkchoiceState(ctx, 2*params.BeaconConfig().SlotsPerEpoch+1, [32]byte{'c'}, blk1.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk2))
	target, err = f.TargetRootForEpoch(blk2.Root(), 2)
	require.NoError(t, err)
	require.Equal(t, target, blk1.Root())

	state, blk3, err := prepareForkchoiceState(ctx, 2*params.BeaconConfig().SlotsPerEpoch+2, [32]byte{'d'}, blk2.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk3))
	target, err = f.TargetRootForEpoch(blk2.Root(), 2)
	require.NoError(t, err)
	require.Equal(t, target, blk1.Root())

	// Prune finalization
	s := f.store
	s.finalizedCheckpoint.Root = blk1.Root()
	require.NoError(t, s.prune(ctx))
	target, err = f.TargetRootForEpoch(blk1.Root(), 1)
	require.NoError(t, err)
	require.Equal(t, [32]byte{}, target)

	// Insert a block for next epoch (slot 0 present)

	state, blk4, err := prepareForkchoiceState(ctx, 3*params.BeaconConfig().SlotsPerEpoch, [32]byte{'e'}, blk1.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk4))
	target, err = f.TargetRootForEpoch(blk4.Root(), 3)
	require.NoError(t, err)
	require.Equal(t, target, blk4.Root())

	state, blk5, err := prepareForkchoiceState(ctx, 3*params.BeaconConfig().SlotsPerEpoch+1, [32]byte{'f'}, blk4.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk5))
	target, err = f.TargetRootForEpoch(blk5.Root(), 3)
	require.NoError(t, err)
	require.Equal(t, target, blk4.Root())

	// Target root where the target epoch is same or ahead of the block slot
	target, err = f.TargetRootForEpoch(blk5.Root(), 4)
	require.NoError(t, err)
	require.Equal(t, target, blk5.Root())

	// Target root where the target epoch is two epochs ago
	target, err = f.TargetRootForEpoch(blk5.Root(), 2)
	require.NoError(t, err)
	require.Equal(t, blk1.Root(), target) // the parent of root4 in epoch 3 is root 1 in epoch 1

	// Target root where the target is two epochs ago, slot 0 was missed
	state, blk6, err := prepareForkchoiceState(ctx, 4*params.BeaconConfig().SlotsPerEpoch+1, [32]byte{'g'}, blk5.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk6))
	target, err = f.TargetRootForEpoch(blk6.Root(), 4)
	require.NoError(t, err)
	require.Equal(t, target, blk5.Root())
	target, err = f.TargetRootForEpoch(blk6.Root(), 2)
	require.NoError(t, err)
	require.Equal(t, target, blk1.Root())

	// Prune finalization
	s.finalizedCheckpoint.Root = blk4.Root()
	require.NoError(t, s.prune(ctx))
	target, err = f.TargetRootForEpoch(blk4.Root(), 3)
	require.NoError(t, err)
	require.Equal(t, blk4.Root(), target)
}

func TestStore_CleanupInserting(t *testing.T) {
	f := setup(0, 0)
	ctx := context.Background()
	st, blk, err := prepareForkchoiceState(ctx, 1, indexToHash(1), indexToHash(2), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NotNil(t, f.InsertNode(ctx, st, blk))
	require.Equal(t, false, f.HasNode(blk.Root()))
}
