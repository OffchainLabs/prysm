// Package beacon-chain defines the entire runtime of an Ethereum beacon node.
package main

import (
	"context"
	"os"
	runtimeDebug "runtime/debug"

	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/runner"
)

func main() {
	defer func() {
		if x := recover(); x != nil {
			log.Errorf("Runtime panic: %v\n%v", x, string(runtimeDebug.Stack()))
			panic(x) // lint:nopanic -- This is just resurfacing the original panic.
		}
	}()

	if err := runner.Run(context.Background(), os.Args); err != nil {
		log.Error(err.Error())
	}
}
