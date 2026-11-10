package historycmd

import (
	"bytes"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/cmd"
	"github.com/OffchainLabs/prysm/v7/cmd/validator/flags"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/io/file"
	"github.com/OffchainLabs/prysm/v7/validator/accounts/userprompt"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

// Reads an input slashing protection EIP-3076
// standard JSON file and attempts to insert its data into our validator DB.
//
// Steps:
// 1. Parse a path to the validator's datadir from the CLI context.
// 2. Read the JSON file from user input.
// 3. Open the validator database, creating it if none exists yet.
// 4. Call the function which actually imports the data from
// the standard slashing protection JSON file into our database.
func importSlashingProtectionJSON(cliCtx *cli.Context) error {
	var err error

	// Check if a minimal database is requested
	db := newDatabase(cliCtx.Bool(features.EnableMinimalSlashingProtection.Name))

	// Get the data directory from the CLI context.
	dataDir := cliCtx.String(cmd.DataDirFlag.Name)
	if !cliCtx.IsSet(cmd.DataDirFlag.Name) {
		dataDir, err = userprompt.InputDirectory(cliCtx, userprompt.DataDirDirPromptText, cmd.DataDirFlag)
		if err != nil {
			return errors.Wrapf(err, "could not read directory value from input")
		}
	}

	// Get the path to the slashing protection JSON file from the CLI context.
	protectionFilePath, err := userprompt.InputDirectory(cliCtx, userprompt.SlashingProtectionJSONPromptText, flags.SlashingProtectionJSONFileFlag)
	if err != nil {
		return fmt.Errorf("input directory: %w", err)
	}

	if protectionFilePath == "" {
		return fmt.Errorf(
			"no path to a slashing protection file specified, please retry or you can also specify it with the %s flag",
			flags.SlashingProtectionJSONFileFlag.Name,
		)
	}

	// Read the JSON file from user input.
	enc, err := file.ReadFileAsBytes(protectionFilePath)
	if err != nil {
		return fmt.Errorf("read file as bytes: %w", err)
	}

	// Look for an existing database under the specified directory or its subdirectories.
	found, dbDir, err := db.find(dataDir)
	if err != nil {
		return errors.Wrapf(err, "error finding validator database at path %s", dataDir)
	}

	// If no database exists yet (typically on a new machine), create it in dataDir, where the validator client will look for it.
	msg := "No validator database found, creating a new one"
	if found {
		dataDir = dbDir
		msg = "Found validator database"
	}

	log.WithField("path", dataDir).Info(msg)

	// Open the validator database.
	valDB, err := db.open(cliCtx.Context, dataDir)
	if err != nil {
		return errors.Wrapf(err, "could not access validator database at path: %s", dataDir)
	}

	// Close the database when we're done.
	defer func() {
		if err := valDB.Close(); err != nil {
			log.WithError(err).Errorf("Could not close validator DB")
		}
	}()

	// Import the data from the standard slashing protection JSON file into our database.
	log.WithField("file", protectionFilePath).Info("Starting import of slashing protection file")
	buf := bytes.NewBuffer(enc)

	if err := valDB.ImportStandardProtectionJSON(cliCtx.Context, buf); err != nil {
		return errors.Wrapf(err, "could not import slashing protection JSON file %s", protectionFilePath)
	}

	log.WithField("path", dataDir).Info("Slashing protection JSON successfully imported")

	return nil
}
