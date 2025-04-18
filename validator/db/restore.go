package db

import (
	"os"
	"path"
	"strings"

	"github.com/OffchainLabs/prysm/v6/cmd"
	"github.com/OffchainLabs/prysm/v6/io/file"
	"github.com/OffchainLabs/prysm/v6/io/prompt"
	"github.com/OffchainLabs/prysm/v6/validator/db/kv"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

const dbExistsYesNoPrompt = "A database file already exists in the target directory. " +
	"Are you sure that you want to overwrite it? [y/n]"

// Restore a Prysm validator database.
func Restore(cliCtx *cli.Context) error {
	sourceFile := cliCtx.String(cmd.RestoreSourceFileFlag.Name)
	targetDir := cliCtx.String(cmd.RestoreTargetDirFlag.Name)

	dbFilePath := path.Join(targetDir, kv.ProtectionDbFileName)
	exists, err := file.Exists(dbFilePath, file.Regular)
	if err != nil {
		return errors.Wrapf(err, "could not check if file exists at %s", dbFilePath)
	}

	if exists {
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
	if err := file.MkdirAll(targetDir); err != nil {
		return err
	}
	if err := file.CopyFile(sourceFile, path.Join(targetDir, kv.ProtectionDbFileName)); err != nil {
		return err
	}

	log.Info("Restore completed successfully")
	return nil
}
