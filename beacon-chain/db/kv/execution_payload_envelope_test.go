package kv

import (
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
			Payload: &enginev1.ExecutionPayloadDeneb{
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
			BlobKzgCommitments: [][]byte{
				bytesutil.PadTo([]byte("commitment1"), 48),
			},
			StateRoot: bytesutil.PadTo([]byte("envelopestateroot"), 32),
		},
		Signature: bytesutil.PadTo([]byte("sig"), 96),
	}
}

func TestStore_SaveAndRetrieveExecutionPayloadEnvelope(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	env := testEnvelope(t)

	// Use the block hash as lookup key (matches what save extracts internally).
	blockHash := bytesutil.ToBytes32(env.Message.Payload.BlockHash)

	// Initially should not exist.
	assert.Equal(t, false, db.HasExecutionPayloadEnvelope(ctx, blockHash))

	// Save (always blinds internally).
	require.NoError(t, db.SaveExecutionPayloadEnvelope(ctx, env))

	// Should exist now.
	assert.Equal(t, true, db.HasExecutionPayloadEnvelope(ctx, blockHash))

	// Load and verify it's always blinded.
	loaded, err := db.ExecutionPayloadEnvelope(ctx, blockHash)
	require.NoError(t, err)

	// Verify metadata is preserved.
	assert.Equal(t, env.Message.Slot, loaded.Message.Slot)
	assert.Equal(t, env.Message.BuilderIndex, loaded.Message.BuilderIndex)
	assert.DeepEqual(t, env.Message.BeaconBlockRoot, loaded.Message.BeaconBlockRoot)
	assert.DeepEqual(t, env.Message.StateRoot, loaded.Message.StateRoot)
	assert.DeepEqual(t, env.Signature, loaded.Signature)

	// PayloadRoot should be 32 bytes (the hash tree root of the full payload).
	assert.Equal(t, 32, len(loaded.Message.PayloadRoot))
	assert.Equal(t, false, bytesutil.ToBytes32(loaded.Message.PayloadRoot) == [32]byte{})
}

func TestStore_DeleteExecutionPayloadEnvelope(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	env := testEnvelope(t)
	blockHash := bytesutil.ToBytes32(env.Message.Payload.BlockHash)

	require.NoError(t, db.SaveExecutionPayloadEnvelope(ctx, env))
	assert.Equal(t, true, db.HasExecutionPayloadEnvelope(ctx, blockHash))

	require.NoError(t, db.DeleteExecutionPayloadEnvelope(ctx, blockHash))
	assert.Equal(t, false, db.HasExecutionPayloadEnvelope(ctx, blockHash))
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

func TestBlindEnvelope_PreservesPayloadRoot(t *testing.T) {
	env := testEnvelope(t)

	blinded, err := blindEnvelope(env)
	require.NoError(t, err)

	// Compute expected payload root.
	expectedRoot, err := env.Message.Payload.HashTreeRoot()
	require.NoError(t, err)

	assert.DeepEqual(t, expectedRoot[:], blinded.Message.PayloadRoot)
	assert.Equal(t, env.Message.BuilderIndex, blinded.Message.BuilderIndex)
	assert.Equal(t, env.Message.Slot, blinded.Message.Slot)
	assert.DeepEqual(t, env.Message.BeaconBlockRoot, blinded.Message.BeaconBlockRoot)
	assert.DeepEqual(t, env.Signature, blinded.Signature)
}
