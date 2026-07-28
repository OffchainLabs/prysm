//go:build !bazel

package bazel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestBuiltWithBazel(t *testing.T) {
	require.Equal(t, false, BuiltWithBazel())
}

func TestModuleRoot(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		root, err := moduleRoot()
		require.NoError(t, err)

		_, err = os.Stat(filepath.Join(root, "go.mod"))
		require.NoError(t, err, "moduleRoot() = %q, but it does not contain go.mod", root)

		// The module root must be an ancestor of (or equal to) the current
		// working directory, which is this package's directory.
		cwd, err := os.Getwd()
		require.NoError(t, err)

		rel, err := filepath.Rel(root, cwd)
		require.NoError(t, err)
		require.Equal(t, false, rel == ".." || filepath.IsAbs(rel),
			"moduleRoot() = %q is not an ancestor of cwd %q (rel=%q)", root, cwd, rel)
	})

	t.Run("not found", func(t *testing.T) {
		// Run from a temporary directory that has no go.mod anywhere above it, so
		// the walk reaches the filesystem root without finding one. t.Chdir restores
		// the working directory when the subtest finishes.
		t.Chdir(t.TempDir())

		_, err := moduleRoot()
		require.ErrorContains(t, "could not locate go.mod", err)
	})
}

func TestTestdataRoot(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		root, err := moduleRoot()
		require.NoError(t, err)

		want := filepath.Join(root, "third_party", "testdata")
		require.Equal(t, want, testdataRoot())
	})

	t.Run("not found", func(t *testing.T) {
		// With no go.mod above the working directory, moduleRoot fails and
		// testdataRoot falls back to the empty string. t.Chdir restores the
		// working directory when the subtest finishes.
		t.Chdir(t.TempDir())

		require.Equal(t, "", testdataRoot())
	})
}

func TestRunfile(t *testing.T) {
	t.Run("absolute", func(t *testing.T) {
		abs := filepath.Join(t.TempDir(), "file.txt")
		require.NoError(t, os.WriteFile(abs, []byte("hi"), 0o600))

		got, err := Runfile(abs)
		require.NoError(t, err)
		require.Equal(t, abs, got)
	})

	t.Run("absolute missing", func(t *testing.T) {
		abs := filepath.Join(t.TempDir(), "does-not-exist.txt")
		_, err := Runfile(abs)
		require.NotNil(t, err, "Runfile(%q) = nil error, want error for a missing absolute path", abs)
	})

	t.Run("relative", func(t *testing.T) {
		// go.mod lives at the module root, which is one of the search bases.
		got, err := Runfile("go.mod")
		require.NoError(t, err)
		require.Equal(t, "go.mod", filepath.Base(got))

		_, err = os.Stat(got)
		require.NoError(t, err, "Runfile(\"go.mod\") = %q, which does not exist", got)
	})

	t.Run("relative missing", func(t *testing.T) {
		// A relative path that exists in none of the bases and maps to no external
		// archive must return an error without attempting a (network) fetch.
		const path = "no_such_dir_zzz/missing.txt"
		_, err := Runfile(path)
		require.NotNil(t, err, "Runfile(%q) = nil error, want error", path)
	})

	t.Run("fetched on miss", func(t *testing.T) {
		// A path that is initially absent but maps to a known archive is resolved
		// after the on-demand fetch materializes it. Run from a temp module root
		// and stub the fetch so it creates the file instead of hitting the network.
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0o600))
		t.Chdir(root)

		// "external/<repo>/..." maps to the <repo> archive, so fetchOnMiss runs.
		const path = "external/some_repo/file.txt"
		want := filepath.Join(root, path)

		orig := fetchArchive
		t.Cleanup(func() { fetchArchive = orig })
		fetchArchive = func(string) error {
			require.NoError(t, os.MkdirAll(filepath.Dir(want), 0o755))
			return os.WriteFile(want, []byte("fetched"), 0o600)
		}

		got, err := Runfile(path)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})
}

func TestRunfilesPath(t *testing.T) {
	got, err := RunfilesPath()
	require.NoError(t, err)

	want, err := moduleRoot()
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestListRunfiles(t *testing.T) {
	t.Run("no test-data root", func(t *testing.T) {
		// With no go.mod above the working directory, testdataRoot is empty and
		// there is nothing to walk.
		t.Chdir(t.TempDir())

		_, err := ListRunfiles()
		require.ErrorContains(t, "could not locate test-data root", err)
	})

	t.Run("missing test-data root", func(t *testing.T) {
		// The module root is found, but it has no third_party/testdata directory.
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0o600))
		t.Chdir(root)

		_, err := ListRunfiles()
		require.ErrorContains(t, "os stat", err)
	})

	t.Run("populated", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0o600))
		td := filepath.Join(root, "third_party", "testdata")
		require.NoError(t, os.MkdirAll(filepath.Join(td, "sub"), 0o755))
		file := filepath.Join(td, "sub", "file.txt")
		require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
		t.Chdir(root)

		entries, err := ListRunfiles()
		require.NoError(t, err)
		require.Equal(t, 1, len(entries))
		require.Equal(t, file, entries[0].Path)
		// ShortPath is relative to the test-data root, using forward slashes.
		require.Equal(t, "sub/file.txt", entries[0].ShortPath)
		require.Equal(t, "", entries[0].Workspace)
	})
}

func TestTestTmpDir(t *testing.T) {
	require.Equal(t, os.TempDir(), TestTmpDir())
}

func TestNewTmpDir(t *testing.T) {
	dir, err := NewTmpDir("bazel-test-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	info, err := os.Stat(dir)
	require.NoError(t, err, "NewTmpDir() = %q, which does not exist", dir)
	require.Equal(t, true, info.IsDir(), "NewTmpDir() = %q, which is not a directory", dir)
	require.StringContains(t, "bazel-test-", filepath.Base(dir))
}

func TestRelativeTestTargetPath(t *testing.T) {
	require.Equal(t, "", RelativeTestTargetPath())
}
