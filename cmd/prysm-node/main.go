// Package prysm-node provides a busybox-style Prysm node binary.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	beaconrunner "github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/runner"
	"github.com/OffchainLabs/prysm/v7/cmd/prysm-node/dispatcher"
	"github.com/OffchainLabs/prysm/v7/cmd/prysm-node/gethrunner"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := dispatcher.Run(ctx, os.Args, dispatcher.Config{
		BeaconChain: beaconrunner.Run,
		Geth:        gethrunner.Run,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
