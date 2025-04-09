package db

import (
	"context"
	"path"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/cmd"
	"github.com/prysmaticlabs/prysm/v5/io/file"
	"github.com/prysmaticlabs/prysm/v5/validator/db/kv"
	"github.com/urfave/cli/v2"
)

// MigrateUp for a validator database.
func MigrateUp(cliCtx *cli.Context) error {
	dataDir := cliCtx.String(cmd.DataDirFlag.Name)

	dbFilePath := path.Join(dataDir, kv.ProtectionDbFileName)
	exists, err := file.Exists(dbFilePath, file.Regular)
	if err != nil {
		return errors.Wrapf(err, "could not check if file exists: %s", dbFilePath)
	}

	if !exists {
		return errors.New("No validator db found at path, nothing to migrate")
	}

	ctx := context.Background()
	log.Info("Opening DB")
	validatorDB, err := kv.NewKVStore(ctx, dataDir, &kv.Config{})
	if err != nil {
		return err
	}
	log.Info("Running migrations")
	return validatorDB.RunUpMigrations(ctx)
}

// MigrateDown for a validator database.
func MigrateDown(cliCtx *cli.Context) error {
	dataDir := cliCtx.String(cmd.DataDirFlag.Name)

	dbFilePath := path.Join(dataDir, kv.ProtectionDbFileName)
	exists, err := file.Exists(dbFilePath, file.Regular)
	if err != nil {
		return errors.Wrapf(err, "could not check if file exists: %s", dbFilePath)
	}

	if !exists {
		return errors.New("No validator db found at path, nothing to rollback")
	}

	ctx := context.Background()
	log.Info("Opening DB")
	validatorDB, err := kv.NewKVStore(ctx, dataDir, &kv.Config{})
	if err != nil {
		return err
	}
	log.Info("Running migrations")
	return validatorDB.RunDownMigrations(ctx)
}
