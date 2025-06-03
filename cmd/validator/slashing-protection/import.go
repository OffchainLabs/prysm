package historycmd

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/OffchainLabs/prysm/v6/cmd"
	"github.com/OffchainLabs/prysm/v6/cmd/validator/flags"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/io/file"
	"github.com/OffchainLabs/prysm/v6/validator/accounts/userprompt"
	"github.com/OffchainLabs/prysm/v6/validator/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/validator/db/iface"
	"github.com/OffchainLabs/prysm/v6/validator/db/kv"
	"github.com/pkg/errors"
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
	var matchPath string
	if isDatabaseMinimal {
		found, matchPath, err = file.RecursiveDirFind(filesystem.DatabaseDirName, dataDir)
	} else {
		found, matchPath, err = file.RecursiveFileFind(kv.ProtectionDbFileName, dataDir)
	}

	if err != nil {
		return errors.Wrapf(err, "error finding validator database at path %s", dataDir)
	}
	if !found {
		databaseFileDir := kv.ProtectionDbFileName
		if isDatabaseMinimal {
			databaseFileDir = filesystem.DatabaseDirName
		}
		return fmt.Errorf("%s (validator database) was not found at path %s, so nothing to export", databaseFileDir, dataDir)
	} else {
		if !isDatabaseMinimal {
			matchPath = filepath.Dir(matchPath) // strip the file name
		}
		dataDir = matchPath
		log.Infof("Found validator database at path %s", dataDir)
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
