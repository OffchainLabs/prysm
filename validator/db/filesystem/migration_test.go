package filesystem

import (
	"context"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestStore_RunUpMigrations(t *testing.T) {
	// Just check `NewStore` does not return an error.
	store, err := NewStore(t.TempDir(), nil)
	require.NoError(t, err, "NewStore should not return an error")

	// Just check `RunUpMigrations` does not return an error.
	err = store.RunUpMigrations(context.Background())
	require.NoError(t, err, "RunUpMigrations should not return an error")
}

func TestStore_RunDownMigrations(t *testing.T) {
	// Just check `NewStore` does not return an error.
	store, err := NewStore(t.TempDir(), nil)
	require.NoError(t, err, "NewStore should not return an error")

	// Just check `RunDownMigrations` does not return an error.
	err = store.RunDownMigrations(context.Background())
	require.NoError(t, err, "RunUpMigrations should not return an error")
}
