package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestStagePbgo(t *testing.T) {
	t.Run("copies .pb.go files, skipping minimal and non-pb.go", func(t *testing.T) {
		dir := t.TempDir()
		pkgDir := filepath.Join(dir, "pkg")
		require.NoError(t, os.MkdirAll(pkgDir, 0o700))
		write := func(name, content string) {
			require.NoError(t, os.WriteFile(filepath.Join(pkgDir, name), []byte(content), 0o600))
		}
		write("foo.pb.go", "package foo\n")
		write("bar.pb.go", "package bar\n")
		write("baz.minimal.pb.go", "package baz\n") // excluded
		write("types.go", "package x\n")            // excluded
		write("README.txt", "nope\n")               // excluded

		stageDir := filepath.Join(dir, "stage")
		require.NoError(t, stagePbgo(pkgDir, stageDir))

		entries, err := os.ReadDir(stageDir)
		require.NoError(t, err)
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		// os.ReadDir returns entries sorted by name.
		require.DeepEqual(t, []string{"bar.pb.go", "foo.pb.go"}, names)

		got, err := os.ReadFile(filepath.Join(stageDir, "foo.pb.go"))
		require.NoError(t, err)
		require.Equal(t, "package foo\n", string(got))
	})

	t.Run("errors when the stage dir cannot be created", func(t *testing.T) {
		dir := t.TempDir()
		// A file where a parent dir is expected makes MkdirAll fail.
		blocker := filepath.Join(dir, "blocker")
		require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

		err := stagePbgo(dir, filepath.Join(blocker, "stage"))
		require.ErrorContains(t, "mkdirAll:", err)
	})

	t.Run("errors when the package dir cannot be read", func(t *testing.T) {
		dir := t.TempDir()
		err := stagePbgo(filepath.Join(dir, "missing"), filepath.Join(dir, "stage"))
		require.ErrorContains(t, "readDir:", err)
	})
}
