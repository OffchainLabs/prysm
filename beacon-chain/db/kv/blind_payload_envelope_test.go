package kv

import (
	"context"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util/random"
)

func TestStore_SignedBlindPayloadEnvelope(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	_, err := db.SignedBlindPayloadEnvelope(ctx, []byte("test"))
	require.ErrorIs(t, err, ErrNotFound)

	env := random.SignedBlindPayloadEnvelope(t)
	err = db.SaveBlindPayloadEnvelope(ctx, env)
	require.NoError(t, err)
	got, err := db.SignedBlindPayloadEnvelope(ctx, env.Message.BeaconBlockRoot)
	require.NoError(t, err)
	require.DeepEqual(t, got, env)
}
