package filesystem

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestStore_RunUpMigrations(t *testing.T) {
	// Just check `NewStore` does not return an error.
	store, err := NewStore(t.TempDir(), nil)
	require.NoError(t, err, "NewStore should not return an error")

	// Just check `RunUpMigrations` does not return an error.
	err = store.RunUpMigrations(t.Context())
	require.NoError(t, err, "RunUpMigrations should not return an error")
}

func TestStore_RunDownMigrations(t *testing.T) {
	// Just check `NewStore` does not return an error.
	store, err := NewStore(t.TempDir(), nil)
	require.NoError(t, err, "NewStore should not return an error")

	// Just check `RunDownMigrations` does not return an error.
	err = store.RunDownMigrations(t.Context())
	require.NoError(t, err, "RunUpMigrations should not return an error")
}
