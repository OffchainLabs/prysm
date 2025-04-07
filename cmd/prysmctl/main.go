package main

import (
	"os"

	"github.com/OffchainLabs/prysm/v6/cmd/prysmctl/checkpointsync"
	"github.com/OffchainLabs/prysm/v6/cmd/prysmctl/db"
	"github.com/OffchainLabs/prysm/v6/cmd/prysmctl/p2p"
	"github.com/OffchainLabs/prysm/v6/cmd/prysmctl/testnet"
	"github.com/OffchainLabs/prysm/v6/cmd/prysmctl/validator"
	"github.com/OffchainLabs/prysm/v6/cmd/prysmctl/weaksubjectivity"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var prysmctlCommands []*cli.Command

func main() {
	app := &cli.App{
		Commands: prysmctlCommands,
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func init() {
	prysmctlCommands = append(prysmctlCommands, checkpointsync.Commands...)
	prysmctlCommands = append(prysmctlCommands, db.Commands...)
	prysmctlCommands = append(prysmctlCommands, p2p.Commands...)
	prysmctlCommands = append(prysmctlCommands, testnet.Commands...)
	prysmctlCommands = append(prysmctlCommands, weaksubjectivity.Commands...)
	prysmctlCommands = append(prysmctlCommands, validator.Commands...)
}
