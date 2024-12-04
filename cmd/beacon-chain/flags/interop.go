package flags

import (
	"github.com/urfave/cli/v2"
)

var (
	// InteropMockEth1DataVotesFlag enables mocking the eth1 proof-of-work chain data put into blocks by proposers.
	InteropMockEth1DataVotesFlag = &cli.BoolFlag{
		Name:  "interop-eth1data-votes",
		Usage: "Enable mocking of eth1 data votes for proposers to package into blocks",
	}
)
