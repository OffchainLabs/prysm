package testing

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/io/file"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/db/filesystem"
	"github.com/prysmaticlabs/prysm/v5/validator/db/iface"
	"github.com/prysmaticlabs/prysm/v5/validator/db/kv"
)

func TestClearDB(t *testing.T) {
	for _, isSlashingProtectionMinimal := range []bool{false, true} {
		t.Run(fmt.Sprintf("slashing protection minimal: %v", isSlashingProtectionMinimal), func(t *testing.T) {
			// Setting up manually is required, since SetupDB() will also register a teardown procedure.
			var (
				testDB iface.ValidatorDB
				err    error
			)

			if isSlashingProtectionMinimal {
				testDB, err = filesystem.NewStore(t.TempDir(), &filesystem.Config{
					PubKeys: nil,
				})
			} else {
				testDB, err = kv.NewKVStore(context.Background(), t.TempDir(), &kv.Config{
					PubKeys: nil,
				})
			}

			require.NoError(t, err, "Failed to instantiate DB")
			require.NoError(t, testDB.ClearDB())

			databaseName := kv.ProtectionDbFileName
			if isSlashingProtectionMinimal {
				databaseName = filesystem.DatabaseDirName
			}

			databasePath := filepath.Join(testDB.DatabasePath(), databaseName)
			exists, err := file.Exists(databasePath, file.Regular)
			require.NoError(t, err, "Failed to check if DB exists")
			require.Equal(t, false, exists, "DB was not cleared")
		})
	}
}
