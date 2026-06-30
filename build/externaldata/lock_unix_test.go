//go:build unix

package externaldata

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestAcquireLock(t *testing.T) {
	t.Run("acquire then release", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "lock")

		l, err := acquireLock(path)
		require.NoError(t, err)
		require.NotNil(t, l)

		_, err = os.Stat(path)
		require.NoError(t, err, "lock file should have been created")

		l.release()
	})

	t.Run("open error", func(t *testing.T) {
		// The parent directory does not exist, so the lock file cannot be opened.
		path := filepath.Join(t.TempDir(), "missing", "lock")

		_, err := acquireLock(path)
		require.ErrorContains(t, "opening lock file", err)
	})

	t.Run("is exclusive until released", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "lock")

		first, err := acquireLock(path)
		require.NoError(t, err)

		// A second acquisition of the same path blocks until the first is released.
		acquired := make(chan *fileLock, 1)
		go func() {
			if l, err := acquireLock(path); err == nil {
				acquired <- l
			}
		}()

		select {
		case <-acquired:
			t.Fatal("second acquireLock succeeded while the lock was held")
		case <-time.After(100 * time.Millisecond):
		}

		first.release()

		select {
		case l := <-acquired:
			l.release()
		case <-time.After(2 * time.Second):
			t.Fatal("second acquireLock did not obtain the lock after release")
		}
	})
}
