package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestSelectTargets(t *testing.T) {
	t.Run("default is presubmit", func(t *testing.T) {
		label, targets, err := selectTargets(nil)
		require.NoError(t, err)
		require.Equal(t, string(suitePresubmit), label)
		require.DeepEqual(t, suites[suitePresubmit], targets)
	})

	t.Run("flags are ignored, default used", func(t *testing.T) {
		// Leading-dash args are flags, not target names; the default suite applies.
		label, targets, err := selectTargets([]string{"-v", "-count=1"})
		require.NoError(t, err)
		require.Equal(t, string(suitePresubmit), label)
		require.DeepEqual(t, suites[suitePresubmit], targets)
	})

	t.Run("suite by name", func(t *testing.T) {
		label, targets, err := selectTargets([]string{string(suitePostsubmit)})
		require.NoError(t, err)
		require.Equal(t, string(suitePostsubmit), label)
		require.DeepEqual(t, suites[suitePostsubmit], targets)
	})

	t.Run("single public kind", func(t *testing.T) {
		label, targets, err := selectTargets([]string{string(kindMinimal)})
		require.NoError(t, err)
		require.Equal(t, string(kindMinimal), label)
		require.DeepEqual(t, []kind{kindMinimal}, targets)
	})

	t.Run("unknown target", func(t *testing.T) {
		_, _, err := selectTargets([]string{"bogus"})
		require.ErrorContains(t, "unknown e2e target", err)
	})
}

func TestEnv(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		t.Setenv("E2E_TEST_VAR", "value")
		require.Equal(t, "value", env("E2E_TEST_VAR", "fallback"))
	})

	t.Run("unset falls back", func(t *testing.T) {
		require.Equal(t, "fallback", env("E2E_TEST_VAR_UNSET_ZZZ", "fallback"))
	})
}

func TestSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	require.NoError(t, os.WriteFile(src, []byte("payload"), 0o600))

	require.NoError(t, symlink(src, dst))

	got, err := os.Readlink(dst)
	require.NoError(t, err)
	require.Equal(t, src, got)
}

func TestIsTerminal(t *testing.T) {
	// A regular file is not a character device.
	f, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })

	require.Equal(t, false, isTerminal(f))
}
