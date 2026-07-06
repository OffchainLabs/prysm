package doublylinkedtree

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestForkChoice_GetAttesterHead(t *testing.T) {
	ctx := t.Context()

	t.Run("nil head returns zero root", func(t *testing.T) {
		f := setup(0, 0)
		f.store.headNode = nil
		require.Equal(t, [32]byte{}, f.GetAttesterHead())
	})

	t.Run("head without parent returns head root", func(t *testing.T) {
		f := setup(0, 0)
		f.store.headNode = f.store.treeRootNode
		require.Equal(t, params.BeaconConfig().ZeroHash, f.GetAttesterHead())
	})

	t.Run("head satisfying inclusion list returns head root", func(t *testing.T) {
		f := setup(0, 0)
		st, blk, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, st, blk))

		head := f.store.nodeByRoot[indexToHash(1)]
		require.Equal(t, false, head.notSatisfyingInclusionList)
		f.store.headNode = head
		require.Equal(t, indexToHash(1), f.GetAttesterHead())
	})

	t.Run("head not satisfying inclusion list returns parent root", func(t *testing.T) {
		f := setup(0, 0)
		st, blk, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, st, blk))

		head := f.store.nodeByRoot[indexToHash(1)]
		head.notSatisfyingInclusionList = true
		f.store.headNode = head
		require.Equal(t, params.BeaconConfig().ZeroHash, f.GetAttesterHead())
	})

	// Covers the wiring in store.insert: a block marked as not satisfying the
	// inclusion list must propagate the flag onto its forkchoice node.
	t.Run("insert propagates not-satisfying flag from block", func(t *testing.T) {
		f := setup(0, 0)
		st, blk, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
		require.NoError(t, err)
		blk.Block().MarkInclusionListNotSatisfied()
		require.NoError(t, f.InsertNode(ctx, st, blk))

		head := f.store.nodeByRoot[indexToHash(1)]
		require.Equal(t, true, head.notSatisfyingInclusionList)
		f.store.headNode = head
		require.Equal(t, params.BeaconConfig().ZeroHash, f.GetAttesterHead())
	})
}
