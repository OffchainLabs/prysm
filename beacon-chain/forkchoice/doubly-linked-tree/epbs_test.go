package doublylinkedtree

import (
	"context"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestFullInsertionPreEPBS(t *testing.T) {
	ctx := context.Background()
	f := setup(1, 1)
	require.Equal(t, 1, f.NodeCount())

	payloadHash := [32]byte{'A'}
	state, blk, err := prepareForkchoiceState(ctx, f, 1, [32]byte{'a'}, params.BeaconConfig().ZeroHash, payloadHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	node, ok := f.store.emptyNodeByRoot[blk.Root()]
	require.Equal(t, true, ok)
	require.Equal(t, blk.Root(), node.block.root)

	fullNode, ok := f.store.fullNodeByPayload[payloadHash]
	require.Equal(t, true, ok)
	require.Equal(t, payloadHash, fullNode.block.payloadHash)
}
