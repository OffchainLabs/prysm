package kv

import (
	"context"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util/random"
)

func Test_SignedExecutionPayloadHeader(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	b := random.SignedBeaconBlock(t)
	blk, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	blockRoot, err := blk.Block().HashTreeRoot()
	require.NoError(t, err)

	require.NoError(t, db.SaveBlock(ctx, blk))
	retrievedHeader, err := db.SignedExecutionPayloadHeader(ctx, blockRoot)
	require.NoError(t, err)
	wantedHeader, err := blk.Block().Body().SignedExecutionPayloadHeader()
	require.NoError(t, err)
	require.DeepEqual(t, wantedHeader, retrievedHeader)
}
