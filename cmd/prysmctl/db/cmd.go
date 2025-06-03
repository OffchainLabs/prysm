package db

import (
	"github.com/OffchainLabs/prysm/v6/cmd/prysmctl/flags"
	"github.com/urfave/cli/v2"
)

var Commands = []*cli.Command{
	{
		Name:  "db",
		Usage: "commands to work with the prysm beacon db",
		Flags: []cli.Flag{
			flags.ForkFlag,
		},
		Subcommands: []*cli.Command{
			queryCmd,
			bucketsCmd,
			spanCmd,
		},
	},
}
