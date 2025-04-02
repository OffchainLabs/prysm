package historycmd

import (
	"bytes"
	"fmt"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/cmd"
	"github.com/prysmaticlabs/prysm/v5/cmd/validator/flags"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	"github.com/prysmaticlabs/prysm/v5/io/file"
	"github.com/prysmaticlabs/prysm/v5/validator/accounts/userprompt"
	"github.com/prysmaticlabs/prysm/v5/validator/db/filesystem"
	"github.com/prysmaticlabs/prysm/v5/validator/db/iface"
	"github.com/prysmaticlabs/prysm/v5/validator/db/kv"
	"github.com/urfave/cli/v2"
)

// Reads an input slashing protection EIP-3076
// standard JSON file and attempts to insert its data into our validator DB.
//
// Steps:
// 1. Parse a path to the validator's datadir from the CLI context.
// 2. Open the validator database.
// 3. Read the JSON file from user input.
// 4. Call the function which actually imports the data from
// the standard slashing protection JSON file into our database.
func importSlashingProtectionJSON(cliCtx *cli.Context) error {
	var (
		valDB iface.ValidatorDB
		found bool
		err   error
	)

	// Check if a minimal database is requested
	isDatabaseMinimal := cliCtx.Bool(features.EnableMinimalSlashingProtection.Name)

	// Get the data directory from the CLI context.
	dataDir := cliCtx.String(cmd.DataDirFlag.Name)
	if !cliCtx.IsSet(cmd.DataDirFlag.Name) {
		dataDir, err = userprompt.InputDirectory(cliCtx, userprompt.DataDirDirPromptText, cmd.DataDirFlag)
		if err != nil {
			return errors.Wrapf(err, "could not read directory value from input")
		}
	}

	// Ensure that the database is found under the specified directory or its subdirectories
	if isDatabaseMinimal {
		found, _, err = file.RecursiveDirFind(filesystem.DatabaseDirName, dataDir)
	} else {
		found, _, err = file.RecursiveFileFind(kv.ProtectionDbFileName, dataDir)
	}

	if err != nil {
		return errors.Wrapf(err, "error finding validator database at path %s", dataDir)
	}

	message := "Found existing database inside of %s"
	if !found {
		message = "Did not find existing database inside of %s, creating a new one"
	}

	log.Infof(message, dataDir)

	// Open the validator database.
	if isDatabaseMinimal {
		valDB, err = filesystem.NewStore(dataDir, nil)
	} else {
		valDB, err = kv.NewKVStore(cliCtx.Context, dataDir, nil)
	}

	if err != nil {
		return errors.Wrapf(err, "could not access validator database at path: %s", dataDir)
	}

	// Close the database when we're done.
	defer func() {
		if err := valDB.Close(); err != nil {
			log.WithError(err).Errorf("Could not close validator DB")
		}
	}()

	// Get the path to the slashing protection JSON file from the CLI context.
	protectionFilePath, err := userprompt.InputDirectory(cliCtx, userprompt.SlashingProtectionJSONPromptText, flags.SlashingProtectionJSONFileFlag)
	if err != nil {
		return errors.Wrap(err, "could not get slashing protection json file")
	}
	if protectionFilePath == "" {
		return fmt.Errorf(
			"no path to a slashing_protection.json file specified, please retry or "+
				"you can also specify it with the %s flag",
			flags.SlashingProtectionJSONFileFlag.Name,
		)
	}

	// Read the JSON file from user input.
	enc, err := file.ReadFileAsBytes(protectionFilePath)
	if err != nil {
		return err
	}

	// Import the data from the standard slashing protection JSON file into our database.
	log.Infof("Starting import of slashing protection file %s", protectionFilePath)
	buf := bytes.NewBuffer(enc)

	if err := valDB.ImportStandardProtectionJSON(cliCtx.Context, buf); err != nil {
		return errors.Wrapf(err, "could not import slashing protection JSON file %s", protectionFilePath)
	}

	log.Infof("Slashing protection JSON successfully imported into %s", dataDir)

	return nil
}
