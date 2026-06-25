package helpers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestWaitForTextInFileWithTimeout_Found(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.log")
	f, err := os.Create(tmp)
	require.NoError(t, err)

	// Write the match text before calling the function.
	_, err = f.WriteString("line one\nRunning bootnode\nline three\n")
	require.NoError(t, err)
	require.NoError(t, f.Sync())

	err = WaitForTextInFileWithTimeout(f, "Running bootnode", 5*time.Second)
	require.NoError(t, err)
}

func TestWaitForTextInFileWithTimeout_Timeout(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.log")
	f, err := os.Create(tmp)
	require.NoError(t, err)

	// Write text that does NOT match.
	_, err = f.WriteString("nothing relevant here\n")
	require.NoError(t, err)
	require.NoError(t, f.Sync())

	err = WaitForTextInFileWithTimeout(f, "will never appear", 1*time.Second)
	require.ErrorContains(t, "could not find requested text", err)
}

func TestWaitForTextInFileWithTimeout_DelayedWrite(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.log")
	f, err := os.Create(tmp)
	require.NoError(t, err)

	// Write the match text after a short delay — exercises the polling loop.
	go func() {
		time.Sleep(600 * time.Millisecond) // Just over one polling interval.
		_, _ = f.WriteString("beacon started\n")
		_ = f.Sync()
	}()

	err = WaitForTextInFileWithTimeout(f, "beacon started", 5*time.Second)
	require.NoError(t, err)
}

func TestWaitForTextInFile_UsesDefaultTimeout(t *testing.T) {
	// WaitForTextInFile delegates to WaitForTextInFileWithTimeout with maxPollingWaitTime (60s).
	// We verify it works by checking a file with existing content.
	tmp := filepath.Join(t.TempDir(), "test.log")
	f, err := os.Create(tmp)
	require.NoError(t, err)

	_, err = f.WriteString("Chain started in sync service\n")
	require.NoError(t, err)
	require.NoError(t, f.Sync())

	err = WaitForTextInFile(f, "Chain started in sync service")
	require.NoError(t, err)
}
