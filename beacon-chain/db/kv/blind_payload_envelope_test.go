package kv

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util/random"
)

func TestStore_SignedBlindPayloadEnvelope(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	_, err := db.SignedBlindPayloadEnvelope(ctx, []byte("test"))
	require.ErrorIs(t, err, ErrNotFound)

	env := random.SignedExecutionPayloadEnvelope(t)
	e, err := blocks.WrappedROSignedExecutionPayloadEnvelope(env)
	require.NoError(t, err)
	err = db.SaveBlindPayloadEnvelope(ctx, e)
	require.NoError(t, err)
	got, err := db.SignedBlindPayloadEnvelope(ctx, env.Message.Payload.BlockHash)
	require.NoError(t, err)
	require.DeepEqual(t, got, env.Blind())
}
