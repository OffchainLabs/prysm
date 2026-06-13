//go:build unix

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// shutdownSignals are the signals on which we tear down a running devnet.
var shutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

// setNewProcGroup makes cmd (the `go test` invocation) the leader of a fresh process
// group, so its whole subtree — the test binary and the beacon/validator/geth/bootnode
// it launches — can be killed together. This also detaches it from the terminal's
// foreground group, which is why a signal handler (see main) is required to forward
// Ctrl-C as a group kill.
func setNewProcGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcGroup SIGKILLs the process group led by pid (Setpgid makes the group id equal
// the leader pid). Best-effort: reaps devnet children the test may have orphaned, e.g.
// when `go test` exits via its -timeout panic without the harness's graceful cleanup.
func killProcGroup(pid int) {
	if pid > 0 {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
}
