package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// writeLogsRepo lays out a fake repo under a temp dir and chdirs into it.
func writeLogsRepo(t *testing.T, files map[string]string) {
	t.Helper()

	dir := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}

	t.Chdir(dir)
}

const logrusSrc = "package foo\n\nimport \"github.com/sirupsen/logrus\"\n"

func TestLogsScan(t *testing.T) {
	writeLogsRepo(t, map[string]string{
		"pkg/foo.go":          logrusSrc,         // match
		"pkg/bar.go":          "package foo\n",   // same dir, reported once
		"other/baz.go":        "package other\n", // no logrus
		"other/qux_test.go":   logrusSrc,         // tests are skipped
		"testing/foo.go":      logrusSrc,         // excluded dir
		"tools/sub/foo.go":    logrusSrc,         // excluded subtree
		".hidden/foo.go":      logrusSrc,         // hidden dir
		"pkg/notgo.txt":       logrusSrc,         // not Go
		"deep/nest/here.go":   logrusSrc,         // match, nested
		"deep/nest/README.md": "github.com/sirupsen/logrus\n",
	})

	files, dirs, err := logsScan()
	require.NoError(t, err)
	require.DeepEqual(t, []string{"deep/nest", "pkg"}, dirs)
	// Scanned files: every non-test, non-hidden .go file outside excluded dirs.
	require.DeepEqual(t, []string{"deep/nest/here.go", "other/baz.go", "pkg/bar.go", "pkg/foo.go"}, files)
}

func TestLogsPkgName(t *testing.T) {
	t.Run("prefers a non-generated, non-test file", func(t *testing.T) {
		writeLogsRepo(t, map[string]string{
			"pkg/log.go":      "package stale\n",
			"pkg/zoo_test.go": "package wrong_test\n",
			"pkg/zoo.go":      "package right\n",
		})

		got, err := logsPkgName("pkg")
		require.NoError(t, err)
		require.Equal(t, "right", got)
	})

	t.Run("falls back to log.go when it is the only Go file", func(t *testing.T) {
		writeLogsRepo(t, map[string]string{"pkg/log.go": "package only\n"})

		got, err := logsPkgName("pkg")
		require.NoError(t, err)
		require.Equal(t, "only", got)
	})

	t.Run("ignores a package clause that is not at the start of a line", func(t *testing.T) {
		writeLogsRepo(t, map[string]string{"pkg/foo.go": "// package nope\npackage yes // comment\n"})

		got, err := logsPkgName("pkg")
		require.NoError(t, err)
		require.Equal(t, "yes", got)
	})

	t.Run("returns an empty name when no file declares a package", func(t *testing.T) {
		writeLogsRepo(t, map[string]string{"pkg/foo.go": "// nothing here\n"})

		got, err := logsPkgName("pkg")
		require.NoError(t, err)
		require.Equal(t, "", got)
	})

	t.Run("errors when the dir cannot be read", func(t *testing.T) {
		writeLogsRepo(t, nil)

		_, err := logsPkgName("missing")
		require.ErrorContains(t, "readDir", err)
	})
}

func TestGenLogs(t *testing.T) {
	t.Run("writes log.go for logrus packages only", func(t *testing.T) {
		writeLogsRepo(t, map[string]string{
			"pkg/foo.go":   logrusSrc,
			"other/bar.go": "package other\n",
		})

		require.NoError(t, genLogs())

		got, err := os.ReadFile(filepath.Join("pkg", logsFileName))
		require.NoError(t, err)
		require.Equal(t, logsFileContent("foo", "pkg"), string(got))

		_, err = os.Stat(filepath.Join("other", logsFileName))
		require.Equal(t, true, os.IsNotExist(err))
	})

	t.Run("overwrites a stale log.go and is idempotent", func(t *testing.T) {
		writeLogsRepo(t, map[string]string{
			"pkg/foo.go": logrusSrc,
			"pkg/log.go": "package foo\n\n// hand-edited\n",
		})

		require.NoError(t, genLogs())
		first, err := os.ReadFile(filepath.Join("pkg", logsFileName))
		require.NoError(t, err)
		require.Equal(t, logsFileContent("foo", "pkg"), string(first))

		info, err := os.Stat(filepath.Join("pkg", logsFileName))
		require.NoError(t, err)

		// A second run leaves the file untouched, so the mtime does not move.
		require.NoError(t, genLogs())
		again, err := os.Stat(filepath.Join("pkg", logsFileName))
		require.NoError(t, err)
		require.Equal(t, info.ModTime(), again.ModTime())
	})
}

func TestLogsExcluded(t *testing.T) {
	require.Equal(t, true, logsExcluded("testing"))
	require.Equal(t, true, logsExcluded("testing/util"))
	require.Equal(t, true, logsExcluded("beacon-chain/p2p/testing"))
	require.Equal(t, false, logsExcluded("testingnot"))
	require.Equal(t, false, logsExcluded("beacon-chain/p2p"))
}
