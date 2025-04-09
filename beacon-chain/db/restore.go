package db

import (
	"os"
	"path"
	"strings"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/kv"
	"github.com/prysmaticlabs/prysm/v5/cmd"
	"github.com/prysmaticlabs/prysm/v5/io/file"
	"github.com/prysmaticlabs/prysm/v5/io/prompt"
	"github.com/urfave/cli/v2"
)

const dbExistsYesNoPrompt = "A database file already exists in the target directory. " +
	"Are you sure that you want to overwrite it? [y/n]"

// Restore a beacon chain database.
func Restore(cliCtx *cli.Context) error {
	sourceFile := cliCtx.String(cmd.RestoreSourceFileFlag.Name)
	targetDir := cliCtx.String(cmd.RestoreTargetDirFlag.Name)

	restoreDir := path.Join(targetDir, kv.BeaconNodeDbDirName)
	restoreFile := path.Join(restoreDir, kv.DatabaseFileName)

	dbExists, err := file.Exists(restoreFile, file.Regular)
	if err != nil {
		return errors.Wrapf(err, "could not check if database exists in %s", restoreFile)
	}

	if dbExists {
		resp, err := prompt.ValidatePrompt(
			os.Stdin, dbExistsYesNoPrompt, prompt.ValidateYesOrNo,
		)
		if err != nil {
			return errors.Wrap(err, "could not validate choice")
		}
		if strings.EqualFold(resp, "n") {
			log.Info("Restore aborted")
			return nil
		}
	}
	if err := file.MkdirAll(restoreDir); err != nil {
		return err
	}
	if err := file.CopyFile(sourceFile, path.Join(restoreDir, kv.DatabaseFileName)); err != nil {
		return err
	}

	log.Info("Restore completed successfully")
	return nil
}
