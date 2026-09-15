package helpers

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestBlockRootFromState(t *testing.T) {
	ctx := t.Context()

	// Build a real block and the state it produces, so the expected root is the block's own root
	// rather than something derived the same way as the code under test.
	genesis, keys := util.DeterministicGenesisState(t, 64)
	blk, err := util.GenerateFullBlock(genesis, keys, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	postState, err := transition.ExecuteStateTransition(ctx, genesis, wsb)
	require.NoError(t, err)
	blockRoot, err := wsb.Block().HashTreeRoot()
	require.NoError(t, err)

	got, err := BlockRootFromState(ctx, postState.Copy())
	require.NoError(t, err)
	require.Equal(t, blockRoot, got)

	// Hashing the header as stored does not produce the block root.
	asStored, err := postState.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)
	require.NotEqual(t, blockRoot, asStored)

}
