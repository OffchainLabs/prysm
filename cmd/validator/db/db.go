package db

import (
	"github.com/prysmaticlabs/prysm/v5/cmd"
	"github.com/prysmaticlabs/prysm/v5/runtime/tos"
	validatordb "github.com/prysmaticlabs/prysm/v5/validator/db"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var log = logrus.WithField("prefix", "db")

var (
	// SourceDataDirFlag defines a path on disk where source Prysm databases are stored. Used for conversion.
	SourceDataDirFlag = &cli.StringFlag{
		Name:     "source-data-dir",
		Usage:    "Source data directory",
		Required: true,
	}

	// SourceDataDirFlag defines a path on disk where source Prysm databases are stored. Used for conversion.
	TargetDataDirFlag = &cli.StringFlag{
		Name:     "target-data-dir",
		Usage:    "Target data directory",
		Required: true,
	}
)

// Commands for interacting with the Prysm validator database.
var Commands = &cli.Command{
	Name:     "db",
	Category: "db",
	Usage:    "Defines commands for interacting with the Prysm validator database.",
	Subcommands: []*cli.Command{
		{
			Name:        "restore",
			Description: `restores a database from a backup file`,
			Flags: cmd.WrapFlags([]cli.Flag{
				cmd.RestoreSourceFileFlag,
				cmd.RestoreTargetDirFlag,
			}),
			Before: tos.VerifyTosAcceptedOrPrompt,
			Action: func(cliCtx *cli.Context) error {
				if err := validatordb.Restore(cliCtx); err != nil {
					log.WithError(err).Fatal("Could not restore database")
				}
				return nil
			},
		},
		{
			Name:     "migrate",
			Category: "db",
			Usage:    "Defines commands for running validator database migrations",
			Subcommands: []*cli.Command{
				{
					Name:  "up",
					Usage: "Runs up migrations for the validator database",
					Flags: cmd.WrapFlags([]cli.Flag{
						cmd.DataDirFlag,
					}),
					Before: tos.VerifyTosAcceptedOrPrompt,
					Action: func(cliCtx *cli.Context) error {
						if err := validatordb.MigrateUp(cliCtx); err != nil {
							log.WithError(err).Fatal("Could not run database migrations")
						}
						return nil
					},
				},
				{
					Name:  "down",
					Usage: "Runs down migrations for the validator database",
					Flags: cmd.WrapFlags([]cli.Flag{
						cmd.DataDirFlag,
					}),
					Before: tos.VerifyTosAcceptedOrPrompt,
					Action: func(cliCtx *cli.Context) error {
						if err := validatordb.MigrateDown(cliCtx); err != nil {
							log.WithError(err).Fatal("Could not run database migrations")
						}
						return nil
					},
				},
			},
		},
		{
			Name:     "convert-complete-to-minimal",
			Category: "db",
			Usage:    "Convert a complete EIP-3076 slashing protection to a minimal one",
			Flags: []cli.Flag{
				SourceDataDirFlag,
				TargetDataDirFlag,
			},
			Before: func(cliCtx *cli.Context) error {
				return cmd.LoadFlagsFromConfig(cliCtx, cliCtx.Command.Flags)
			},
			Action: func(cliCtx *cli.Context) error {
				sourcedDatabasePath := cliCtx.String(SourceDataDirFlag.Name)
				targetDatabasePath := cliCtx.String(TargetDataDirFlag.Name)

				// Convert the database
				err := validatordb.ConvertDatabase(cliCtx.Context, sourcedDatabasePath, targetDatabasePath, false)
				if err != nil {
					log.WithError(err).Fatal("Could not convert database")
				}

				return nil
			},
		},
	},
}
