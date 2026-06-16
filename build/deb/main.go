package main

import (
	"fmt"
	"os"

	"github.com/OffchainLabs/prysm/v7/build/debpkg"
)

func main() {
	if err := debpkg.ConfigFromEnv().Build(); err != nil {
		fmt.Fprintln(os.Stderr, "❌ deb:", err)
		os.Exit(1)
	}
}
