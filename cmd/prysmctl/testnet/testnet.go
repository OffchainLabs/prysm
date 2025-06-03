package testnet

import (
	"github.com/OffchainLabs/prysm/v6/cmd/prysmctl/flags"
	"github.com/urfave/cli/v2"
)

var Commands = []*cli.Command{
	{
		Name:  "testnet",
		Usage: "commands for dealing with Ethereum beacon chain testnets",
		Flags: []cli.Flag{
			flags.ForkFlag,
		},
		Subcommands: []*cli.Command{
			generateGenesisStateCmd,
		},
	},
}
