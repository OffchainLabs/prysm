// prysm-vet runs Prysm's static analysis (the analyzers of Bazel's nogo, with the same
// nogo_config.json). It is a `go vet` / `go fix` tool:
//
//	go vet -vettool=$(which prysm-vet) ./...
//	go fix -fixtool=$(which prysm-vet) ./...
//
// Use `make lint [fix]` rather than calling it directly.
package main

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/tools/go/analysis/unitchecker"
)

func main() {
	configs, err := parseConfigs(rawConfig)
	if err != nil {
		fatal(err)
	}

	analyzers, err := allAnalyzers()
	if err != nil {
		fatal(err)
	}

	// Outside a module (e.g. the handshake `go vet` runs from a temp dir), filters match the
	// absolute path, which is harmless: no diagnostic is reported there.
	root := ""
	if wd, err := os.Getwd(); err == nil {
		root, _ = moduleRoot(wd)
	}

	if err := install(analyzers, configs, root); err != nil {
		fatal(err)
	}

	deps := depsEnabled(os.Args[1:])
	if deps && fixEnabled(os.Args[1:]) {
		// `go fix` only collects the fixes of the listed packages: the dependencies would write
		// theirs in place, possibly concurrently with a listed package fixing the same file.
		fatal(errors.New("-deps cannot be used with -fix"))
	}

	if err := rewriteConfig(os.Args, deps); err != nil {
		fatal(err)
	}

	unitchecker.Main(analyzers...)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "prysm-vet:", err)
	os.Exit(1)
}
