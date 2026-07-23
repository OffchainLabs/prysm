package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestLoadCache(t *testing.T) {
	// writeCache writes raw JSON to cacheFile under a temp dir and chdirs there.
	writeCache := func(t *testing.T, raw string) {
		t.Helper()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, cacheFile), []byte(raw), 0o600))
		t.Chdir(dir)
	}

	t.Run("returns an empty cache when the file is absent", func(t *testing.T) {
		t.Chdir(t.TempDir())
		got := loadCache()
		require.Equal(t, cacheVersion, got.Version)
		require.Equal(t, 0, len(got.Kinds))
	})

	t.Run("loads a valid cache", func(t *testing.T) {
		writeCache(t, `{"version":1,"kinds":{"proto":"abc","ssz":"def"}}`)
		got := loadCache()
		require.Equal(t, cacheVersion, got.Version)
		require.DeepEqual(t, map[string]string{"proto": "abc", "ssz": "def"}, got.Kinds)
	})

	t.Run("discards a cache with a mismatched version", func(t *testing.T) {
		writeCache(t, `{"version":999,"kinds":{"proto":"abc"}}`)
		got := loadCache()
		require.Equal(t, cacheVersion, got.Version)
		require.Equal(t, 0, len(got.Kinds))
	})
}

func TestStoreCache(t *testing.T) {
	t.Chdir(t.TempDir())

	want := genCache{Kinds: map[string]string{"mocks": "deadbeef"}}
	require.NoError(t, storeCache(want))

	got := loadCache()
	require.Equal(t, cacheVersion, got.Version)
	require.DeepEqual(t, map[string]string{"mocks": "deadbeef"}, got.Kinds)
}

func TestManifest(t *testing.T) {
	// setupRepo lays out a minimal repo (go.mod/go.sum, build/gen, proto tree)
	// sufficient for manifest(kindProto), and chdirs into it.
	setupRepo := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		write := func(rel, content string) {
			path := filepath.Join(dir, filepath.FromSlash(rel))
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
			require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		}
		write("go.mod", "module x\n")
		write("go.sum", "")
		write("build/gen/main.go", "package main\n")
		write("proto/foo.proto", "syntax = \"proto3\";\n")
		write(sszProtoLibraryBzl, "mainnet = {}\n")
		t.Chdir(dir)

		return dir
	}

	setupRepo(t)
	first, err := manifest(kindProto)
	require.NoError(t, err)
	second, err := manifest(kindProto)
	require.NoError(t, err)
	require.Equal(t, first, second)
}

func TestFileSum(t *testing.T) {
	t.Run("hashes an existing file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "f.txt")
		content := []byte("hello world\n")
		require.NoError(t, os.WriteFile(path, content, 0o600))

		got, err := fileSum(path)
		require.NoError(t, err)

		want := sha256.Sum256(content)
		require.Equal(t, hex.EncodeToString(want[:]), got)
	})

	t.Run("returns \"absent\" for a missing file", func(t *testing.T) {
		got, err := fileSum(filepath.Join(t.TempDir(), "missing"))
		require.NoError(t, err)
		require.Equal(t, "absent", got)
	})

	t.Run("errors when the path cannot be read", func(t *testing.T) {
		// A directory cannot be read with os.ReadFile (not a NotExist error).
		got, err := fileSum(t.TempDir())
		require.ErrorContains(t, "readFile", err)
		require.Equal(t, "", got)
	})
}

func TestSpecificFiles(t *testing.T) {
	// writeRepo lays out the given <rel path>:<content> entries under a temp dir
	// and chdirs in.
	writeRepo := func(t *testing.T, files map[string]string) {
		t.Helper()
		dir := t.TempDir()
		for rel, content := range files {
			path := filepath.Join(dir, filepath.FromSlash(rel))
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
			require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		}
		t.Chdir(dir)
	}

	t.Run("proto delegates to protoFiles", func(t *testing.T) {
		writeRepo(t, map[string]string{
			"proto/foo.proto":  "syntax = \"proto3\";\n",
			sszProtoLibraryBzl: "mainnet = {}\n",
		})

		got, err := specificFiles(kindProto)
		require.NoError(t, err)

		want, err := protoFiles()
		require.NoError(t, err)
		require.DeepEqual(t, want, got)
	})

	t.Run("ssz delegates to sszFiles", func(t *testing.T) {
		writeRepo(t, map[string]string{
			"proto/foo/service.pb.go": "package foo\n",
			"proto/foo/BUILD.bazel": `
	ssz_gen_marshal(
	    name = "ssz_foo",
	    out = "x.ssz.go",
	    objs = ["X"],
	)
	`,
		})

		got, err := specificFiles(kindSSZ)
		require.NoError(t, err)

		want, err := sszFiles()
		require.NoError(t, err)
		require.DeepEqual(t, want, got)
	})

	t.Run("mocks delegates to mockFiles", func(t *testing.T) {
		writeRepo(t, map[string]string{
			"proto/prysm/v1alpha1/beacon.go":  "package eth\n",
			"validator/client/iface/iface.go": "package iface\n",
		})

		got, err := specificFiles(kindMocks)
		require.NoError(t, err)

		want, err := mockFiles()
		require.NoError(t, err)
		require.DeepEqual(t, want, got)
	})

	t.Run("errors on an unknown kind", func(t *testing.T) {
		got, err := specificFiles(kind("bogus"))
		require.IsNil(t, got)
		require.ErrorContains(t, `unknown kind "bogus"`, err)
	})
}

func TestCommonFiles(t *testing.T) {
	dir := t.TempDir()
	genDir := filepath.Join(dir, "build", "gen")
	require.NoError(t, os.MkdirAll(genDir, 0o700))

	write := func(name string) {
		require.NoError(t, os.WriteFile(filepath.Join(genDir, name), []byte("x"), 0o600))
	}

	write("cache.go")
	write("main.go")
	write("BUILD.bazel")                                                    // not a .go file -> excluded
	write("notes.txt")                                                      // excluded
	require.NoError(t, os.MkdirAll(filepath.Join(genDir, "sub.go"), 0o700)) // a dir -> excluded
	t.Chdir(dir)

	got, err := commonFiles()
	require.NoError(t, err)
	require.DeepEqual(t, []string{
		"go.mod",
		"go.sum",
		"build/gen/cache.go",
		"build/gen/main.go",
	}, got)
}

func TestGoPkgFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600))
	}

	write("a.go")
	write("b.go")
	write("c_test.go")                                                   // excluded
	write("d.txt")                                                       // excluded
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub.go"), 0o700)) // dir -> excluded

	got, err := goPkgFiles(dir)
	require.NoError(t, err)
	require.DeepEqual(t, []string{
		filepath.ToSlash(filepath.Join(dir, "a.go")),
		filepath.ToSlash(filepath.Join(dir, "b.go")),
	}, got)
}

func TestProtoFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(rel string) {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
	}
	write("proto/foo.proto")
	write("proto/eth/BUILD.bazel")
	write("proto/eth/service.pb.go")
	write("proto/eth/notes.txt") // excluded
	write("proto/eth/types.go")  // excluded
	write(sszProtoLibraryBzl)    // the .bzl is appended explicitly
	t.Chdir(dir)

	got, err := protoFiles()
	require.NoError(t, err)
	require.DeepEqual(t, []string{
		"proto/eth/BUILD.bazel",
		"proto/eth/service.pb.go",
		"proto/foo.proto",
		sszProtoLibraryBzl,
	}, got)
}

func TestDedupeSorted(t *testing.T) {
	got := dedupeSorted([]string{"c", "a", "b", "a", "c"})
	require.DeepEqual(t, []string{"a", "b", "c"}, got)
}
