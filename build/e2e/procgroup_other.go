//go:build !unix

package main

import (
	"os"
	"os/exec"
)

// Non-unix fallbacks: e2e runs on a unix host (linux/darwin) — these keep the package
// buildable elsewhere without process-group teardown.
var shutdownSignals = []os.Signal{os.Interrupt}

func setNewProcGroup(*exec.Cmd) {}

func killProcGroup(int) {}
