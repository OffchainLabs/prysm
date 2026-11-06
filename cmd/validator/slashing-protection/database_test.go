package historycmd

import (
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/io/file"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/db/filesystem"
	"github.com/OffchainLabs/prysm/v7/validator/db/kv"
)

// createDatabase opens the database in dir and writes to it, so it exists on disk.
func createDatabase(t *testing.T, db database, dir string) {
	validatorDB, err := db.open(t.Context(), dir)
	require.NoError(t, err)
	require.NoError(t, validatorDB.SaveGenesisValidatorsRoot(t.Context(), bytesutil.PadTo([]byte{1}, 32)))
	require.NoError(t, validatorDB.Close())
}

func TestNewDatabase(t *testing.T) {
	testCases := []struct {
		name         string
		isMinimal    bool
		expectedName string
		fileType     file.ObjType
	}{
		{name: "complete", isMinimal: false, expectedName: kv.ProtectionDbFileName, fileType: file.Regular},
		{name: "minimal", isMinimal: true, expectedName: filesystem.DatabaseDirName, fileType: file.Directory},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db := newDatabase(tc.isMinimal)
			require.Equal(t, tc.expectedName, db.name)

			// The database doesn't exist yet: opening it then writing to it creates it.
			dir := filepath.Join(t.TempDir(), "fresh")
			createDatabase(t, db, dir)

			exists, err := file.Exists(filepath.Join(dir, tc.expectedName), tc.fileType)
			require.NoError(t, err)
			require.Equal(t, true, exists)

			// The created database is found.
			found, foundDir, err := db.find(dir)
			require.NoError(t, err)
			require.Equal(t, true, found)
			require.Equal(t, dir, foundDir)
		})
	}
}

func TestDatabaseFind(t *testing.T) {
	for _, isMinimal := range []bool{false, true} {
		db := newDatabase(isMinimal)
		other := newDatabase(!isMinimal)

		t.Run(db.name+"/missing data dir", func(t *testing.T) {
			found, foundDir, err := db.find(filepath.Join(t.TempDir(), "missing"))
			require.NoError(t, err)
			require.Equal(t, false, found)
			require.Equal(t, "", foundDir)
		})

		t.Run(db.name+"/empty data dir", func(t *testing.T) {
			found, foundDir, err := db.find(t.TempDir())
			require.NoError(t, err)
			require.Equal(t, false, found)
			require.Equal(t, "", foundDir)
		})

		t.Run(db.name+"/flat", func(t *testing.T) {
			dataDir := t.TempDir()
			createDatabase(t, db, dataDir)

			found, foundDir, err := db.find(dataDir)
			require.NoError(t, err)
			require.Equal(t, true, found)
			require.Equal(t, dataDir, foundDir)
		})

		t.Run(db.name+"/nested", func(t *testing.T) {
			dataDir := t.TempDir()
			nestedDir := filepath.Join(dataDir, "a", "b")
			createDatabase(t, db, nestedDir)

			found, foundDir, err := db.find(dataDir)
			require.NoError(t, err)
			require.Equal(t, true, found)
			require.Equal(t, nestedDir, foundDir)
		})

		t.Run(db.name+"/other backend only", func(t *testing.T) {
			dataDir := t.TempDir()
			createDatabase(t, other, dataDir)

			found, foundDir, err := db.find(dataDir)
			require.NoError(t, err)
			require.Equal(t, false, found)
			require.Equal(t, "", foundDir)
		})
	}
}
