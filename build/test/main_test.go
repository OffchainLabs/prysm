package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func fakeGo(t *testing.T, stdout, stderr string, exit int) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake go stub is a POSIX shell script")
	}

	dir := t.TempDir()
	outFile := filepath.Join(dir, "out")
	errFile := filepath.Join(dir, "err")
	require.NoError(t, os.WriteFile(outFile, []byte(stdout), 0o644))
	require.NoError(t, os.WriteFile(errFile, []byte(stderr), 0o644))

	path := filepath.Join(dir, "go")
	script := "#!/bin/sh\ncat " + outFile + "\ncat " + errFile + " 1>&2\nexit " + strconv.Itoa(exit) + "\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		done <- b.String()
	}()

	fn()
	require.NoError(t, w.Close())
	os.Stdout = old
	return <-done
}

func TestJoinKinds(t *testing.T) {
	require.Equal(t, "mainnet minimal", joinKinds([]kind{mainnet, minimal}))
	require.Equal(t, "", joinKinds(nil))
}

func TestPassSpec(t *testing.T) {
	t.Run("mainnet", func(t *testing.T) {
		goBin := fakeGo(t, "pkg/a\n", "", 0)
		pkgs, header, tagFlag, err := passSpec(goBin, mainnet)
		require.NoError(t, err)
		require.Equal(t, "=== mainnet pass (excluding spectests) ===", header)
		require.Equal(t, "-tags=develop", tagFlag)
		require.DeepEqual(t, []string{"pkg/a"}, pkgs)
	})

	t.Run("mainnet-spectest", func(t *testing.T) {
		base := "github.com/OffchainLabs/prysm/v7"
		list := strings.Join([]string{
			base + "/testing/spectest/mainnet",
			base + "/testing/spectest/minimal/bar",
			base + "/testing/spectest/shared/common",
		}, "\n")
		goBin := fakeGo(t, list+"\n", "", 0)
		pkgs, header, tagFlag, err := passSpec(goBin, mainnetSpectest)
		require.NoError(t, err)
		require.Equal(t, "=== mainnet spectest pass ===", header)
		require.Equal(t, "-tags=develop", tagFlag)
		// The minimal spec-tests are dropped; they run in the minimal-spectest pass.
		require.DeepEqual(t, []string{
			base + "/testing/spectest/mainnet",
			base + "/testing/spectest/shared/common",
		}, pkgs)
	})

	t.Run("minimal", func(t *testing.T) {
		goBin := fakeGo(t, "pkg/a\npkg/b\n", "", 0)
		pkgs, header, tagFlag, err := passSpec(goBin, minimal)
		require.NoError(t, err)
		require.Equal(t, "=== minimal pass (-tags=minimal, excluding spectests) ===", header)
		require.Equal(t, "-tags=develop,minimal", tagFlag)
		require.DeepEqual(t, []string{"pkg/a", "pkg/b"}, pkgs)
	})

	t.Run("minimal-spectest", func(t *testing.T) {
		goBin := fakeGo(t, "spectest/minimal/a\nspectest/minimal/b\n", "", 0)
		pkgs, header, tagFlag, err := passSpec(goBin, minimalSpectest)
		require.NoError(t, err)
		require.Equal(t, "=== minimal spectest pass (-tags=minimal) ===", header)
		require.Equal(t, "-tags=develop,minimal", tagFlag)
		require.DeepEqual(t, []string{"spectest/minimal/a", "spectest/minimal/b"}, pkgs)
	})

	t.Run("unknown pass errors", func(t *testing.T) {
		_, _, _, err := passSpec("go", kind("bogus"))
		require.ErrorContains(t, "unknown pass", err)
	})
}

func TestSelectKinds(t *testing.T) {
	t.Run("no args runs every pass", func(t *testing.T) {
		got, err := selectKinds(nil, false)
		require.NoError(t, err)
		require.DeepEqual(t, []kind{mainnet, mainnetSpectest, minimal, minimalSpectest}, got)
	})

	t.Run("no args with race runs the mainnet passes", func(t *testing.T) {
		got, err := selectKinds(nil, true)
		require.NoError(t, err)
		require.DeepEqual(t, []kind{mainnet, mainnetSpectest}, got)
	})

	t.Run("explicit passes", func(t *testing.T) {
		got, err := selectKinds([]string{"minimal", "mainnet"}, false)
		require.NoError(t, err)
		require.DeepEqual(t, []kind{minimal, mainnet}, got)
	})

	t.Run("unknown pass errors", func(t *testing.T) {
		_, err := selectKinds([]string{"bogus"}, false)
		require.ErrorContains(t, "not a test pass: bogus", err)
	})
}

func TestStatusIcon(t *testing.T) {
	for _, line := range []string{"✓ pkg/a", "✖ pkg/b", "∅ pkg/c", "↻ pkg/d"} {
		require.Equal(t, true, statusIcon.MatchString(line), "expected %q to match", line)
	}
	require.Equal(t, false, statusIcon.MatchString("Downloading modules"))
}

func TestStreamProgress(t *testing.T) {
	in := "setup line\n✓ pkg/a\n✖ pkg/b\n"
	out := captureStdout(t, func() {
		streamProgress(strings.NewReader(in), 2)
	})

	// Non-status lines pass through; status lines get a right-aligned counter.
	require.Equal(t, "setup line\n[1/2] ✓ pkg/a\n[2/2] ✖ pkg/b\n", out)
}

func TestGoList(t *testing.T) {
	t.Run("parses and trims output", func(t *testing.T) {
		goBin := fakeGo(t, "pkg/a\npkg/b\n\n", "", 0)
		pkgs, err := goList(goBin, "./...")
		require.NoError(t, err)
		require.DeepEqual(t, []string{"pkg/a", "pkg/b"}, pkgs)
	})

	t.Run("reports go list failure with stderr", func(t *testing.T) {
		goBin := fakeGo(t, "", "boom: bad pattern", 1)
		_, err := goList(goBin, "./...")
		require.ErrorContains(t, "go list", err)
		require.ErrorContains(t, "boom: bad pattern", err)
	})

	t.Run("reports failure to start the command", func(t *testing.T) {
		// A non-existent binary fails to start, which is not an *exec.ExitError.
		_, err := goList(filepath.Join(t.TempDir(), "does-not-exist-go"), "./...")
		require.ErrorContains(t, "go list", err)
	})
}

func TestMainnetPackages(t *testing.T) {
	base := "github.com/OffchainLabs/prysm/v7"
	list := strings.Join([]string{
		base + "/beacon-chain/core",
		base + "/testing/endtoend/foo",
		base + "/testing/spectest/minimal/bar",
		base + "/beacon-chain/rpc/prysm/v1alpha1/beacon",
		base + "/beacon-chain/rpc/prysm/v1alpha1/validator",
		base + "/beacon-chain/rpc/prysm/v1alpha1/beacon/blocks",
		base + "/config",
	}, "\n")

	pkgs, err := mainnetPackages(fakeGo(t, list+"\n", "", 0))
	require.NoError(t, err)
	require.DeepEqual(t, []string{
		base + "/beacon-chain/core",
		base + "/beacon-chain/rpc/prysm/v1alpha1/beacon/blocks",
		base + "/config",
	}, pkgs)
}

func TestEnv(t *testing.T) {
	t.Run("returns the value when set", func(t *testing.T) {
		t.Setenv("EXTERNALDATA_TEST_ENV", "custom")
		require.Equal(t, "custom", env("EXTERNALDATA_TEST_ENV", "fallback"))
	})

	t.Run("falls back when unset", func(t *testing.T) {
		require.Equal(t, "fallback", env("EXTERNALDATA_TEST_ENV_UNSET", "fallback"))
	})
}
