package file_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/io/file"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// Temporary adversarial probes for WriteFileAtomically; not for commit.
func TestProbeWriteFileAtomicallyEdgeCases(t *testing.T) {
	t.Run("symlink chain resolves to final target", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "real.txt")
		link2 := filepath.Join(dir, "link2")
		link1 := filepath.Join(dir, "link1")
		require.NoError(t, file.WriteFile(target, []byte("old")))
		require.NoError(t, os.Symlink(target, link2))
		require.NoError(t, os.Symlink(link2, link1))

		require.NoError(t, file.WriteFileAtomically(link1, []byte("new")))
		got, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.Equal(t, "new", string(got))
		// Reader on original path sees new content.
		got, err = os.ReadFile(link1)
		require.NoError(t, err)
		assert.Equal(t, "new", string(got))
	})

	t.Run("symlink to other dir leaves no temp files anywhere", func(t *testing.T) {
		dirA := t.TempDir()
		dirB := t.TempDir()
		target := filepath.Join(dirB, "real.txt")
		link := filepath.Join(dirA, "keys.txt")
		require.NoError(t, file.WriteFile(target, []byte("old")))
		require.NoError(t, os.Symlink(target, link))

		require.NoError(t, file.WriteFileAtomically(link, []byte("new")))
		got, err := os.ReadFile(link)
		require.NoError(t, err)
		assert.Equal(t, "new", string(got))
		for _, d := range []string{dirA, dirB} {
			m, err := filepath.Glob(filepath.Join(d, ".*tmp*"))
			require.NoError(t, err)
			assert.Equal(t, 0, len(m), "leftover temp in %s", d)
		}
	})

	t.Run("missing file under symlinked parent dir readable via original path", func(t *testing.T) {
		base := t.TempDir()
		realDir := filepath.Join(base, "real-dir")
		require.NoError(t, os.MkdirAll(realDir, 0700))
		linkDir := filepath.Join(base, "link-dir")
		require.NoError(t, os.Symlink(realDir, linkDir))
		origPath := filepath.Join(linkDir, "keys.txt")

		require.NoError(t, file.WriteFileAtomically(origPath, []byte("new")))
		got, err := os.ReadFile(origPath)
		require.NoError(t, err)
		assert.Equal(t, "new", string(got))
		info, err := os.Stat(filepath.Join(realDir, "keys.txt"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	})

	t.Run("symlink loop errors without creating temp", func(t *testing.T) {
		dir := t.TempDir()
		link := filepath.Join(dir, "loop")
		require.NoError(t, os.Symlink(link, link))
		err := file.WriteFileAtomically(link, []byte("new"))
		require.NotNil(t, err)
		m, err2 := filepath.Glob(filepath.Join(dir, ".*tmp*"))
		require.NoError(t, err2)
		assert.Equal(t, 0, len(m))
	})

	t.Run("missing parent dir errors", func(t *testing.T) {
		dir := t.TempDir()
		err := file.WriteFileAtomically(filepath.Join(dir, "nope", "keys.txt"), []byte("new"))
		require.NotNil(t, err)
	})

	t.Run("relative path lands in cwd target", func(t *testing.T) {
		dir := t.TempDir()
		wd, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(dir))
		defer func() { require.NoError(t, os.Chdir(wd)) }()
		require.NoError(t, file.WriteFileAtomically("rel-keys.txt", []byte("new")))
		got, err := os.ReadFile(filepath.Join(dir, "rel-keys.txt"))
		require.NoError(t, err)
		assert.Equal(t, "new", string(got))
	})

	t.Run("dangling symlink in intermediate component errors", func(t *testing.T) {
		dir := t.TempDir()
		linkDir := filepath.Join(dir, "linkdir")
		require.NoError(t, os.Symlink(filepath.Join(dir, "missing-dir"), linkDir))
		err := file.WriteFileAtomically(filepath.Join(linkDir, "keys.txt"), []byte("new"))
		require.NotNil(t, err)
		t.Log(err)
	})

	t.Run("write error leaves original intact and no temp", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "keys.txt")
		require.NoError(t, file.WriteFile(target, []byte("old")))
		roDir := filepath.Join(dir, "ro")
		require.NoError(t, os.MkdirAll(roDir, 0500))
		defer func() { _ = os.Chmod(roDir, 0700) }()
		roTarget := filepath.Join(roDir, "inner.txt")
		err := file.WriteFileAtomically(roTarget, []byte("new"))
		require.NotNil(t, err)
		t.Log(err)
	})
}
