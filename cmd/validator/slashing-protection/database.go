package historycmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/OffchainLabs/prysm/v7/io/file"
	"github.com/OffchainLabs/prysm/v7/validator/db/filesystem"
	"github.com/OffchainLabs/prysm/v7/validator/db/iface"
	"github.com/OffchainLabs/prysm/v7/validator/db/kv"
)

// database describes a validator database backend: the complete one or the minimal one.
type database struct {
	// name is the file or directory the backend stores its data in.
	name string

	// findFunc looks for name under a directory or its subdirectories.
	findFunc func(name, dir string) (bool, string, error)

	// open opens the database from the directory containing name, creating it if it doesn't exist yet.
	open func(ctx context.Context, dir string) (iface.ValidatorDB, error)
}

// newDatabase returns the validator database backend to use.
func newDatabase(isDatabaseMinimal bool) database {
	if isDatabaseMinimal {
		return database{
			name:     filesystem.DatabaseDirName,
			findFunc: file.RecursiveDirFind,
			open: func(_ context.Context, dir string) (iface.ValidatorDB, error) {
				store, err := filesystem.NewStore(dir, nil)
				if err != nil {
					return nil, fmt.Errorf("new store: %w", err)
				}

				return store, nil
			},
		}
	}

	return database{
		name:     kv.ProtectionDbFileName,
		findFunc: file.RecursiveFileFind,
		open: func(ctx context.Context, dir string) (iface.ValidatorDB, error) {
			store, err := kv.NewKVStore(ctx, dir, nil)
			if err != nil {
				return nil, fmt.Errorf("new kv store: %w", err)
			}

			return store, nil
		},
	}
}

// find looks for an existing validator database under dataDir or its subdirectories.
// It returns whether one was found and the directory containing it. A missing dataDir means no database.
func (d database) find(dataDir string) (bool, string, error) {
	dataDirExists, err := file.Exists(dataDir, file.Directory)
	if err != nil {
		return false, "", fmt.Errorf("file exists: %w", err)
	}

	if !dataDirExists {
		return false, "", nil
	}

	found, matchPath, err := d.findFunc(d.name, dataDir)
	if err != nil {
		return false, "", fmt.Errorf("find %s: %w", d.name, err)
	}

	if !found {
		return false, "", nil
	}

	// Strip the database file or directory name: both stores expect its parent directory.
	return true, filepath.Dir(matchPath), nil
}
