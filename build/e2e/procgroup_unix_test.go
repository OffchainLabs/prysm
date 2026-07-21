//go:build unix

package main

import (
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestSetNewProcGroup(t *testing.T) {
	cmd := &exec.Cmd{}
	setNewProcGroup(cmd)

	require.NotNil(t, cmd.SysProcAttr)
	require.Equal(t, true, cmd.SysProcAttr.Setpgid)
}

func TestKillProcGroup(t *testing.T) {
	t.Run("non-positive pid is a no-op", func(t *testing.T) {
		// Guards against ever broadcasting to pid 0 (the caller's own group) or -1 (every
		// process). Both must return nil without signalling anything.
		require.NoError(t, killProcGroup(0))
		require.NoError(t, killProcGroup(-1))
	})

	t.Run("kills a running process group", func(t *testing.T) {
		// A child that would otherwise sleep well past the test.
		cmd := exec.Command("sleep", "60")
		setNewProcGroup(cmd)
		require.NoError(t, cmd.Start())

		require.NoError(t, killProcGroup(cmd.Process.Pid))

		waitErr := make(chan error, 1)
		go func() { waitErr <- cmd.Wait() }()

		select {
		case err := <-waitErr:
			// The process was terminated by our signal, so Wait reports failure.
			require.NotNil(t, err)
			var exitErr *exec.ExitError
			require.Equal(t, true, errors.As(err, &exitErr))
			ws, ok := exitErr.Sys().(syscall.WaitStatus)
			require.Equal(t, true, ok)
			require.Equal(t, true, ws.Signaled())
			require.Equal(t, syscall.SIGKILL, ws.Signal())
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			t.Fatal("process group was not killed within 10s")
		}
	})
}
