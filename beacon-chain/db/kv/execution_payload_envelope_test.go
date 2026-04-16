package kv

import (
	"bytes"
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func testEnvelope(t *testing.T) *ethpb.SignedExecutionPayloadEnvelope {
	t.Helper()
	return &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Payload: &enginev1.ExecutionPayloadGloas{
				ParentHash:    bytesutil.PadTo([]byte("parent"), 32),
				FeeRecipient:  bytesutil.PadTo([]byte("fee"), 20),
				StateRoot:     bytesutil.PadTo([]byte("stateroot"), 32),
				ReceiptsRoot:  bytesutil.PadTo([]byte("receipts"), 32),
				LogsBloom:     bytesutil.PadTo([]byte{}, 256),
				PrevRandao:    bytesutil.PadTo([]byte("randao"), 32),
				BlockNumber:   100,
				GasLimit:      30000000,
				GasUsed:       21000,
				Timestamp:     1000,
				ExtraData:     []byte("extra"),
				BaseFeePerGas: bytesutil.PadTo([]byte{1}, 32),
				BlockHash:     bytesutil.PadTo([]byte("blockhash"), 32),
				Transactions:  [][]byte{[]byte("tx1"), []byte("tx2")},
				Withdrawals:   []*enginev1.Withdrawal{{Index: 1, ValidatorIndex: 2, Address: bytesutil.PadTo([]byte("addr"), 20), Amount: 100}},
				BlobGasUsed:   131072,
				ExcessBlobGas: 0,
			},
			ExecutionRequests: &enginev1.ExecutionRequests{},
			BuilderIndex:      primitives.BuilderIndex(42),
			BeaconBlockRoot:   bytesutil.PadTo([]byte("beaconroot"), 32),
			Slot:              primitives.Slot(99),
			StateRoot:         bytesutil.PadTo([]byte("envelopestateroot"), 32),
		},
		Signature: bytesutil.PadTo([]byte("sig"), 96),
	}
}

func TestStore_SaveAndRetrieveExecutionPayloadEnvelope(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	env := testEnvelope(t)

	// Keyed by beacon block root.
	blockRoot := bytesutil.ToBytes32(env.Message.BeaconBlockRoot)

	// Initially should not exist.
	assert.Equal(t, false, db.HasExecutionPayloadEnvelope(ctx, blockRoot))

	// Save (always blinds internally).
	require.NoError(t, db.SaveExecutionPayloadEnvelope(ctx, env))

	// Should exist now.
	assert.Equal(t, true, db.HasExecutionPayloadEnvelope(ctx, blockRoot))

	// Load and verify it's blinded.
	loaded, err := db.ExecutionPayloadEnvelope(ctx, blockRoot)
	require.NoError(t, err)

	// Verify metadata is preserved.
	assert.Equal(t, env.Message.Slot, loaded.Message.Slot)
	assert.Equal(t, env.Message.BuilderIndex, loaded.Message.BuilderIndex)
	assert.DeepEqual(t, env.Message.BeaconBlockRoot, loaded.Message.BeaconBlockRoot)
	assert.DeepEqual(t, env.Message.StateRoot, loaded.Message.StateRoot)
	assert.DeepEqual(t, env.Signature, loaded.Signature)

	// BlockHash should be the payload's block hash (not a hash tree root).
	assert.DeepEqual(t, env.Message.Payload.BlockHash, loaded.Message.BlockHash)
	assert.Equal(t, true, bytes.Equal(env.Message.Payload.ParentHash, loaded.Message.ParentBlockHash))
}

func TestStore_DeleteExecutionPayloadEnvelope(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	env := testEnvelope(t)
	blockRoot := bytesutil.ToBytes32(env.Message.BeaconBlockRoot)

	require.NoError(t, db.SaveExecutionPayloadEnvelope(ctx, env))
	assert.Equal(t, true, db.HasExecutionPayloadEnvelope(ctx, blockRoot))

	require.NoError(t, db.DeleteExecutionPayloadEnvelope(ctx, blockRoot))
	assert.Equal(t, false, db.HasExecutionPayloadEnvelope(ctx, blockRoot))
}

func TestStore_ExecutionPayloadEnvelope_NotFound(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	nonExistent := bytesutil.ToBytes32([]byte("nonexistent"))

	_, err := db.ExecutionPayloadEnvelope(ctx, nonExistent)
	require.ErrorContains(t, "not found", err)
}

func TestStore_SaveExecutionPayloadEnvelope_NilRejected(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	err := db.SaveExecutionPayloadEnvelope(ctx, nil)
	require.ErrorContains(t, "nil", err)
}

func TestStore_ExecutionPayloadEnvelopeByBlockHash(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	env := testEnvelope(t)
	blockHash := bytesutil.ToBytes32(env.Message.Payload.BlockHash)

	// Save envelope — should populate both primary and BlockHash index.
	require.NoError(t, db.SaveExecutionPayloadEnvelope(ctx, env))

	// Look up by block hash.
	loaded, err := db.ExecutionPayloadEnvelopeByBlockHash(ctx, blockHash)
	require.NoError(t, err)
	assert.Equal(t, env.Message.Slot, loaded.Message.Slot)
	assert.DeepEqual(t, env.Message.Payload.BlockHash, loaded.Message.BlockHash)
	assert.Equal(t, true, bytes.Equal(env.Message.Payload.ParentHash, loaded.Message.ParentBlockHash))
}

func TestStore_ExecutionPayloadEnvelopeByBlockHash_NotFound(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	nonExistent := bytesutil.ToBytes32([]byte("nonexistent"))

	_, err := db.ExecutionPayloadEnvelopeByBlockHash(ctx, nonExistent)
	require.ErrorContains(t, "not found", err)
}

func TestStore_DeleteExecutionPayloadEnvelope_CleansBlockHashIndex(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	env := testEnvelope(t)
	blockRoot := bytesutil.ToBytes32(env.Message.BeaconBlockRoot)
	blockHash := bytesutil.ToBytes32(env.Message.Payload.BlockHash)

	require.NoError(t, db.SaveExecutionPayloadEnvelope(ctx, env))

	// Verify BlockHash lookup works before delete.
	_, err := db.ExecutionPayloadEnvelopeByBlockHash(ctx, blockHash)
	require.NoError(t, err)

	// Delete should clean up both buckets.
	require.NoError(t, db.DeleteExecutionPayloadEnvelope(ctx, blockRoot))

	_, err = db.ExecutionPayloadEnvelopeByBlockHash(ctx, blockHash)
	require.ErrorContains(t, "not found", err)
}

func TestBlindEnvelope_PreservesBlockHash(t *testing.T) {
	env := testEnvelope(t)

	blinded := blindEnvelope(env)

	// Should contain the block hash from the payload, not a hash tree root.
	assert.DeepEqual(t, env.Message.Payload.BlockHash, blinded.Message.BlockHash)
	assert.Equal(t, true, bytes.Equal(env.Message.Payload.ParentHash, blinded.Message.ParentBlockHash))

	// Metadata should be preserved.
	assert.Equal(t, env.Message.BuilderIndex, blinded.Message.BuilderIndex)
	assert.Equal(t, env.Message.Slot, blinded.Message.Slot)
	assert.DeepEqual(t, env.Message.BeaconBlockRoot, blinded.Message.BeaconBlockRoot)
	assert.DeepEqual(t, env.Message.StateRoot, blinded.Message.StateRoot)
	assert.DeepEqual(t, env.Signature, blinded.Signature)
}
