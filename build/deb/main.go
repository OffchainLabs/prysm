// Command deb builds the Prysm .deb packages (prysm-beacon-chain, prysm-validator)
// with nfpm, replacing the Bazel rules_pkg pipeline (Phase 6). It (1) cross-builds the
// linux portable binaries via the Phase-4 crossbuild (in-process), then (2) runs
// `go tool nfpm` once per (package × architecture) against the per-package nfpm.yaml.
//
// Driven by `make deb`; run from the repository root. See build/debpkg for the logic.
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
