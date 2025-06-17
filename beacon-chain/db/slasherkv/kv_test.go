package slasherkv

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/require"
)

// setupDB instantiates and returns a Store instance.
func setupDB(t testing.TB) *Store {
	db, err := NewKVStore(t.Context(), t.TempDir())
	require.NoError(t, err, "Failed to instantiate DB")
	t.Cleanup(func() {
		require.NoError(t, db.Close(), "Failed to close database")
	})
	return db
}
