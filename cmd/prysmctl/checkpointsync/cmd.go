package checkpointsync

import (
	"github.com/OffchainLabs/prysm/v6/cmd/prysmctl/flags"
	"github.com/urfave/cli/v2"
)

var Commands = []*cli.Command{
	{
		Name:    "checkpoint-sync",
		Aliases: []string{"cpt-sync"},
		Usage:   "commands for managing checkpoint sync",
		Flags: []cli.Flag{
			flags.ForkFlag,
		},
		Subcommands: []*cli.Command{
			downloadCmd,
		},
	},
}
