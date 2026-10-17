// Package authtoken defines commands for managing the validator client API auth token.
package authtoken

import (
	"github.com/OffchainLabs/prysm/v7/cmd"
	"github.com/OffchainLabs/prysm/v7/cmd/validator/flags"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/runtime/tos"
	"github.com/OffchainLabs/prysm/v7/validator/rpc"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

// Commands for managing the validator client API auth token.
var Commands = &cli.Command{
	Name:        "generate-auth-token",
	Category:    "auth-token",
	Usage:       "Generates an authentication token for the validator client APIs.",
	Description: `Generate an authentication token for the validator client keymanager APIs`,
	Flags: cmd.WrapFlags([]cli.Flag{
		// WalletDirFlag is accepted for backwards compatibility with
		// `validator web generate-auth-token`, but has no effect.
		flags.WalletDirFlag,
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
		// Resolve the token path exactly like the validator client does at startup, so a
		// regenerated token is always the one the running server will read.
		authTokenPath := cliCtx.String(flags.AuthTokenPathFlag.Name)
		if cliCtx.IsSet(flags.WalletDirFlag.Name) {
			log.Warnf(
				"--%s does not affect where the auth token is written. The token is written to --%s (%s), which is also the only path the validator client reads.",
				flags.WalletDirFlag.Name, flags.AuthTokenPathFlag.Name, authTokenPath,
			)
		}
		return errors.Wrap(rpc.CreateAuthToken(authTokenPath), "could not create auth token")
	},
}
