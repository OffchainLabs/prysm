package historycmd

import (
	"github.com/OffchainLabs/prysm/v6/cmd"
	"github.com/OffchainLabs/prysm/v6/cmd/validator/flags"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/runtime/tos"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// Commands for slashing protection.
var Commands = &cli.Command{
	Name:     "slashing-protection-history",
	Category: "slashing-protection-history",
	Usage:    "Defines commands for interacting your validator's slashing protection history.",
	Subcommands: []*cli.Command{
		{
			Name:        "export",
			Description: `exports your validator slashing protection history into an EIP-3076 compliant JSON`,
			Flags: cmd.WrapFlags([]cli.Flag{
				cmd.DataDirFlag,
				flags.SlashingProtectionExportDirFlag,
				features.Mainnet,
				features.SepoliaTestnet,
				features.HoleskyTestnet,
				features.HoodiTestnet,
				features.EnableMinimalSlashingProtection,
				cmd.AcceptTosFlag,
			}),
			Before: func(cliCtx *cli.Context) error {
				if err := cmd.LoadFlagsFromConfig(cliCtx, cliCtx.Command.Flags); err != nil {
					return err
				}
				return tos.VerifyTosAcceptedOrPrompt(cliCtx)
			},
			Action: func(cliCtx *cli.Context) error {
				if err := features.ConfigureValidator(cliCtx); err != nil {
					return err
				}
				if err := exportSlashingProtectionJSON(cliCtx); err != nil {
					logrus.Fatalf("Could not export slashing protection file: %v", err)
				}
				return nil
			},
		},
		{
			Name:        "import",
			Description: `imports a selected EIP-3076 compliant slashing protection JSON to the validator database`,
			Flags: cmd.WrapFlags([]cli.Flag{
				cmd.DataDirFlag,
				flags.SlashingProtectionJSONFileFlag,
				features.Mainnet,
				features.SepoliaTestnet,
				features.HoleskyTestnet,
				features.HoodiTestnet,
				features.EnableMinimalSlashingProtection,
				cmd.AcceptTosFlag,
			}),
			Before: func(cliCtx *cli.Context) error {
				if err := cmd.LoadFlagsFromConfig(cliCtx, cliCtx.Command.Flags); err != nil {
					return err
				}
				return tos.VerifyTosAcceptedOrPrompt(cliCtx)
			},
			Action: func(cliCtx *cli.Context) error {
				if err := features.ConfigureValidator(cliCtx); err != nil {
					return err
				}
				err := importSlashingProtectionJSON(cliCtx)
				if err != nil {
					logrus.Fatalf("Could not import slashing protection cli: %v", err)
				}
				return nil
			},
		},
	},
}
