package doublylinkedtree

import (
	"context"
	"testing"
	"time"

	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

const (
	anchorEpoch          = primitives.Epoch(100)
	anchorJustifiedEpoch = primitives.Epoch(10)
)

// unfinalizedAnchorForkchoice builds a store rooted at an unfinalized checkpoint sync origin.
func unfinalizedAnchorForkchoice(t *testing.T) (*ForkChoice, [32]byte, primitives.Slot) {
	ctx := context.TODO()
	anchorSlot, err := slots.EpochStart(anchorEpoch)
	require.NoError(t, err)
	currentSlot, err := slots.EpochStart(anchorEpoch + 2)
	require.NoError(t, err)

	f := New()
	f.SetBalancesByRooter(func(_ context.Context, _ [32]byte) ([]uint64, error) { return f.justifiedBalances, nil })
	f.SetGenesisTime(time.Now().Add(-params.SlotsDuration(currentSlot, params.BeaconConfig())))

	anchorRoot := indexToHash(1)
	f.store.justifiedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: anchorJustifiedEpoch, Root: anchorRoot}
	f.store.finalizedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: anchorEpoch, Root: anchorRoot}

	st, blk, err := prepareForkchoiceState(ctx, anchorSlot, anchorRoot, [32]byte{}, [32]byte{}, anchorJustifiedEpoch, anchorJustifiedEpoch-2)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))
	require.Equal(t, anchorRoot, f.store.treeRootNode.root)

	return f, anchorRoot, anchorSlot
}

func TestClampCheckpointRoot(t *testing.T) {
	f, anchorRoot, _ := unfinalizedAnchorForkchoice(t)
	unknown := indexToHash(99)

	t.Run("known root is untouched", func(t *testing.T) {
		require.Equal(t, anchorRoot, f.store.clampCheckpointRoot(anchorEpoch, anchorRoot))
	})
	t.Run("unknown root at or below the tree root clamps", func(t *testing.T) {
		require.Equal(t, anchorRoot, f.store.clampCheckpointRoot(anchorEpoch, unknown))
		require.Equal(t, anchorRoot, f.store.clampCheckpointRoot(anchorJustifiedEpoch, unknown))
	})
	t.Run("unknown root above the tree root is untouched", func(t *testing.T) {
		require.Equal(t, unknown, f.store.clampCheckpointRoot(anchorEpoch+1, unknown))
	})
	t.Run("empty tree is untouched", func(t *testing.T) {
		require.Equal(t, unknown, New().store.clampCheckpointRoot(anchorEpoch, unknown))
	})
}

// TestStore_Head_UnfinalizedAnchor is the regression that motivates the truthful justified epoch.
func TestStore_Head_UnfinalizedAnchor(t *testing.T) {
	ctx := context.TODO()
	f, anchorRoot, anchorSlot := unfinalizedAnchorForkchoice(t)

	parent := anchorRoot
	var last [32]byte
	for i := 1; i <= 3; i++ {
		last = indexToHash(uint64(10 + i))
		st, blk, err := prepareForkchoiceState(ctx, anchorSlot+primitives.Slot(i), last, parent, [32]byte{}, anchorJustifiedEpoch, anchorJustifiedEpoch-2)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, st, blk))
		parent = last
	}

	head, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, last, head)

	// Synthesizing the justified epoch forward to the anchor epoch strands the head at the anchor.
	f.store.justifiedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: anchorEpoch, Root: anchorRoot}
	_, err = f.Head(ctx)
	require.ErrorContains(t, "is not eligible", err)
	require.Equal(t, true, f.store.allTipsAreInvalid)
}

func TestStore_Head_JustifiedRootBelowTreeRoot(t *testing.T) {
	f, anchorRoot, _ := unfinalizedAnchorForkchoice(t)

	f.store.justifiedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: anchorJustifiedEpoch, Root: indexToHash(99)}
	head, err := f.store.head(context.TODO())
	require.NoError(t, err)
	require.Equal(t, anchorRoot, head)

	// A root we have never seen from above the tree root is still an error.
	f.store.justifiedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: anchorEpoch + 1, Root: indexToHash(99)}
	_, err = f.store.head(context.TODO())
	assert.ErrorContains(t, errUnknownJustifiedRoot.Error(), err)
}

func TestStore_Prune_FinalizedRootBelowTreeRoot(t *testing.T) {
	f, anchorRoot, _ := unfinalizedAnchorForkchoice(t)

	f.store.finalizedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: anchorEpoch - 1, Root: indexToHash(99)}
	require.NoError(t, f.store.prune(context.TODO()))
	require.Equal(t, anchorRoot, f.store.treeRootNode.root)

	f.store.finalizedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: anchorEpoch + 1, Root: indexToHash(99)}
	require.ErrorIs(t, f.store.prune(context.TODO()), errUnknownFinalizedRoot)
}

// TestForkChoice_InsertNode_JustifiedRootBelowTreeRoot covers a checkpoint justified after the
// anchor that names a block below it, as late attestations can do for the preceding epoch.
func TestForkChoice_InsertNode_JustifiedRootBelowTreeRoot(t *testing.T) {
	ctx := context.TODO()
	f, anchorRoot, anchorSlot := unfinalizedAnchorForkchoice(t)

	child := indexToHash(11)
	st, blk, err := prepareForkchoiceState(ctx, anchorSlot+1, child, anchorRoot, [32]byte{}, anchorEpoch-1, anchorJustifiedEpoch)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	require.Equal(t, true, f.HasNode(child))
	require.Equal(t, anchorRoot, f.store.justifiedCheckpoint.Root)
	require.Equal(t, anchorEpoch-1, f.store.justifiedCheckpoint.Epoch)
}
