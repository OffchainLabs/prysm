package web

import (
	"fmt"
	"path/filepath"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/cmd"
	"github.com/OffchainLabs/prysm/v6/cmd/validator/flags"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/runtime/tos"
	"github.com/OffchainLabs/prysm/v6/validator/rpc"
	"github.com/urfave/cli/v2"
)

// Commands for managing Prysm validator accounts.
var Commands = &cli.Command{
	Name:     "web",
	Category: "web",
	Usage:    "Defines commands for interacting with the Prysm web interface.",
	Subcommands: []*cli.Command{
		{
			Name:        "generate-auth-token",
			Description: `Generate an authentication token for the Prysm web interface`,
			Flags: cmd.WrapFlags([]cli.Flag{
				flags.WalletDirFlag,
				flags.HTTPServerHost,
				flags.HTTPServerPort,
				flags.AuthTokenPathFlag,
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
				walletDirPath := cliCtx.String(flags.WalletDirFlag.Name)
				if walletDirPath == "" {
					log.Fatal("--wallet-dir not specified")
				}
				host := cliCtx.String(flags.HTTPServerHost.Name)
				port := cliCtx.Int(flags.HTTPServerPort.Name)
				validatorWebAddr := fmt.Sprintf("%s:%d", host, port)
				authTokenPath := filepath.Join(walletDirPath, api.AuthTokenFileName)
				tempAuthTokenPath := cliCtx.String(flags.AuthTokenPathFlag.Name)
				if tempAuthTokenPath != "" {
					authTokenPath = tempAuthTokenPath
				}
				if err := rpc.CreateAuthToken(authTokenPath, validatorWebAddr); err != nil {
					log.WithError(err).Fatal("Could not create web auth token")
				}
				return nil
			},
		},
	},
}
