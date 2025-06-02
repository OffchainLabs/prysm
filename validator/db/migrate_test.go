package db

import (
	"flag"
	"testing"

	"github.com/OffchainLabs/prysm/v6/cmd"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	dbtest "github.com/OffchainLabs/prysm/v6/validator/db/testing"
	"github.com/urfave/cli/v2"
)

func TestMigrateUp_NoDBFound(t *testing.T) {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String(cmd.DataDirFlag.Name, "", "")
	require.NoError(t, set.Set(cmd.DataDirFlag.Name, ""))
	cliCtx := cli.NewContext(&app, set, nil)
	err := MigrateUp(cliCtx)
	assert.ErrorContains(t, "No validator db found at path", err)
}

// TestMigrateUp_OK tests that a migration up is successful.
// Migration is not needed nor supported for minimal slashing protection database.
// This, it is tested only for complete slashing protection database.
func TestMigrateUp_OK(t *testing.T) {
	isSlashingProtectionMinimal := false
	validatorDB := dbtest.SetupDB(t, t.TempDir(), nil, isSlashingProtectionMinimal)
	dbPath := validatorDB.DatabasePath()
	require.NoError(t, validatorDB.Close())
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String(cmd.DataDirFlag.Name, dbPath, "")
	require.NoError(t, set.Set(cmd.DataDirFlag.Name, dbPath))
	cliCtx := cli.NewContext(&app, set, nil)
	assert.NoError(t, MigrateUp(cliCtx))
}

func TestMigrateDown_NoDBFound(t *testing.T) {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String(cmd.DataDirFlag.Name, "", "")
	require.NoError(t, set.Set(cmd.DataDirFlag.Name, ""))
	cliCtx := cli.NewContext(&app, set, nil)
	err := MigrateDown(cliCtx)
	assert.ErrorContains(t, "No validator db found at path", err)
}

// TestMigrateUp_OK tests that a migration down is successful.
// Migration is not needed nor supported for minimal slashing protection database.
// This, it is tested only for complete slashing protection database.
func TestMigrateDown_OK(t *testing.T) {
	isSlashingProtectionMinimal := false
	validatorDB := dbtest.SetupDB(t, t.TempDir(), nil, isSlashingProtectionMinimal)
	dbPath := validatorDB.DatabasePath()
	require.NoError(t, validatorDB.Close())
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String(cmd.DataDirFlag.Name, dbPath, "")
	require.NoError(t, set.Set(cmd.DataDirFlag.Name, dbPath))
	cliCtx := cli.NewContext(&app, set, nil)
	assert.NoError(t, MigrateDown(cliCtx))
}
