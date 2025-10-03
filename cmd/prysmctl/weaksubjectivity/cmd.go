package weaksubjectivity

import (
	"github.com/OffchainLabs/prysm/v6/cmd/prysmctl/flags"
	"github.com/urfave/cli/v2"
)

var Commands = []*cli.Command{
	{
		Name:    "weak-subjectivity",
		Aliases: []string{"ws"},
		Usage:   "commands dealing with weak subjectivity",
		Flags: []cli.Flag{
			flags.ForkFlag,
		},
		Subcommands: []*cli.Command{
			checkpointCmd,
		},
	},
}
