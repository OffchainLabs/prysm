package backfill

import "github.com/urfave/cli/v2"

var Commands = []*cli.Command{
	{
		Name:  "backfill",
		Usage: "commands for verifying backfill operations",
		Subcommands: []*cli.Command{
			verifyCmd,
		},
	},
}
