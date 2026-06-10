// Command cross cross-compiles the distributed Prysm binaries for every run-target
// (Phase 4 of the Bazel->Go-toolchain migration). The logic lives in the crossbuild
// package; this command just wires it to the environment the Makefile exports and is
// invoked by `make cross-build`.
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
