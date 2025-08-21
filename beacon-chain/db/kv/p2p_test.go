package kv

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestStore_MetadataSeqNum(t *testing.T) {
	ctx := t.Context()
	db := setupDB(t)

	seqNum, err := db.MetadataSeqNum(ctx)
	require.ErrorIs(t, err, ErrNotFoundMetadataSeqNum)
	assert.Equal(t, uint64(0), seqNum)

	initialSeqNum := uint64(42)
	err = db.SaveMetadataSeqNum(ctx, initialSeqNum)
	require.NoError(t, err)

	retrievedSeqNum, err := db.MetadataSeqNum(ctx)
	require.NoError(t, err)
	assert.Equal(t, initialSeqNum, retrievedSeqNum)

	updatedSeqNum := uint64(43)
	err = db.SaveMetadataSeqNum(ctx, updatedSeqNum)
	require.NoError(t, err)

	retrievedSeqNum, err = db.MetadataSeqNum(ctx)
	require.NoError(t, err)
	assert.Equal(t, updatedSeqNum, retrievedSeqNum)
}
