package main

import (
	"fmt"
	"os"

	"github.com/OffchainLabs/prysm/v7/build/crossbuild"
)

func main() {
	if err := crossbuild.ConfigFromEnv().Build(); err != nil {
		fmt.Fprintln(os.Stderr, "❌ cross:", err)
		os.Exit(1)
	}
}
