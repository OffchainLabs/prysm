package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"
)

func TestProtoPkgDirs(t *testing.T) {
	got := protoPkgDirs(map[string]string{
		"proto/zeta":  modeStock,
		"proto/alpha": modeCast,
		"proto/beta":  modeCastGRPC,
	})

	require.DeepEqual(t, []string{"proto/alpha", "proto/beta", "proto/zeta"}, got)
}

func TestApplyGenModes(t *testing.T) {
	// writeFile creates a file (and parent dirs) at the given perm under dir.
	writeFile := func(t *testing.T, dir, rel string, perm os.FileMode) string {
		t.Helper()
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("package x\n"), perm))

		return path
	}

	dir := t.TempDir()
	// All start at 0600 so the chmod effect is observable.
	pbgo := writeFile(t, dir, "proto/eth/foo.pb.go", 0o600)
	minimal := writeFile(t, dir, "proto/eth/foo.minimal.pb.go", 0o600)
	nested := writeFile(t, dir, "proto/eth/v1/bar.pb.go", 0o600)
	other := writeFile(t, dir, "proto/eth/foo.go", 0o600)

	require.NoError(t, applyGenModes(dir))

	mode := func(path string) os.FileMode {
		info, err := os.Stat(path)
		require.NoError(t, err)

		return info.Mode().Perm()
	}

	require.Equal(t, os.FileMode(0o755), mode(pbgo))
	require.Equal(t, os.FileMode(0o755), mode(nested))
	require.Equal(t, os.FileMode(0o644), mode(minimal))
	// Not a .pb.go file: left at its original mode.
	require.Equal(t, os.FileMode(0o600), mode(other))
}

func TestDownloadVerified(t *testing.T) {
	body := []byte("the archive bytes")
	sum := fmt.Sprintf("%x", sha256.Sum256(body))

	t.Run("returns the body when the checksum matches", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(body)
		}))
		defer srv.Close()

		got, err := downloadVerified(srv.URL, sum)
		require.NoError(t, err)
		require.DeepEqual(t, body, got)
	})

	t.Run("errors on a checksum mismatch", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(body)
		}))
		defer srv.Close()

		got, err := downloadVerified(srv.URL, "deadbeef")
		require.IsNil(t, got)
		require.ErrorContains(t, "sha256 mismatch", err)
	})

	t.Run("errors on a non-200 status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		got, err := downloadVerified(srv.URL, sum)
		require.IsNil(t, got)
		require.ErrorContains(t, "404", err)
	})
}

func TestExtractZipFile(t *testing.T) {
	// zipFile builds an in-memory archive with one entry and returns the
	// corresponding *zip.File.
	zipFile := func(t *testing.T, name, content string) *zip.File {
		t.Helper()
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
		require.NoError(t, zw.Close())

		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		require.NoError(t, err)
		require.Equal(t, 1, len(zr.File))

		return zr.File[0]
	}

	dest := t.TempDir()
	f := zipFile(t, "google/api/http.proto", "syntax = \"proto3\";\n")

	require.NoError(t, extractZipFile(f, dest, "google/api/http.proto"))

	got, err := os.ReadFile(filepath.Join(dest, "google", "api", "http.proto"))
	require.NoError(t, err)
	require.Equal(t, "syntax = \"proto3\";\n", string(got))
}

func TestGenerateNetwork(t *testing.T) {
	// generateNetwork shells out to built protoc plugins once pkgs is non-empty,
	// which needs the Go toolchain — out of scope for a unit test. With an empty
	// pkgs map the plugin loop never runs and compileDescriptors gets zero files,
	// so the staging/compile pipeline is exercised end-to-end without subprocesses.
	t.Run("succeeds with no packages and creates the out dir", func(t *testing.T) {
		root := t.TempDir()
		// stageProtos and buildMMAP walk the repo-relative "proto" tree; one
		// .proto file is enough for the staged proto/ dir to exist.
		require.NoError(t, os.MkdirAll(filepath.Join(root, "proto"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(root, "proto", "foo.proto"), []byte("syntax = \"proto3\";\n"), 0o600))
		t.Chdir(root)

		outDir := filepath.Join(root, "out")
		err := generateNetwork(map[string]string{}, outDir, "", t.TempDir(), map[string]string{})
		require.NoError(t, err)

		info, statErr := os.Stat(outDir)
		require.NoError(t, statErr)
		require.Equal(t, true, info.IsDir())
	})

	t.Run("gathers protos then stops at an unknown plugin mode", func(t *testing.T) {
		root := t.TempDir()
		// A self-contained proto so the gathered files (line that appends
		// pkgProtos(dir)) compile, reaching the per-package plugin loop.
		require.NoError(t, os.MkdirAll(filepath.Join(root, "proto", "foo"), 0o700))
		require.NoError(t, os.WriteFile(
			filepath.Join(root, "proto", "foo", "foo.proto"),
			[]byte("syntax = \"proto3\";\npackage foo;\nmessage Foo { string a = 1; }\n"),
			0o600,
		))
		t.Chdir(root)

		// An unknown mode makes pluginForMode fail before runPlugin shells out,
		// so the test covers the proto-gathering + compile path without a subprocess.
		pkgs := map[string]string{"proto/foo": "bogus-mode"}
		err := generateNetwork(map[string]string{}, filepath.Join(root, "out"), "", t.TempDir(), pkgs)
		require.ErrorContains(t, "pluginForMode:", err)
	})

	t.Run("runs the plugin and writes its output", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "proto", "foo"), 0o700))
		require.NoError(t, os.WriteFile(
			filepath.Join(root, "proto", "foo", "foo.proto"),
			[]byte("syntax = \"proto3\";\npackage foo;\nmessage Foo { string a = 1; }\n"),
			0o600,
		))
		t.Chdir(root)

		// Pre-marshal the response our fake plugin will emit on stdout.
		name := "proto/foo/foo.pb.go"
		content := "package foo\n"

		resp := &pluginpb.CodeGeneratorResponse{
			File: []*pluginpb.CodeGeneratorResponse_File{{
				Name:    &name,
				Content: &content,
			}},
		}
		respBytes, err := proto.Marshal(resp)
		require.NoError(t, err)
		respFile := filepath.Join(root, "resp.bin")
		require.NoError(t, os.WriteFile(respFile, respBytes, 0o600))

		// modeStock makes pluginForMode return <binDir>/protoc-gen-go. Plant a
		// fake there: drain the request on stdin, then emit the canned response.
		binDir := filepath.Join(root, "bin")
		require.NoError(t, os.MkdirAll(binDir, 0o700))
		script := "#!/bin/sh\ncat > /dev/null\ncat " + respFile + "\n"
		require.NoError(t, os.WriteFile(filepath.Join(binDir, "protoc-gen-go"), []byte(script), 0o755)) // #nosec G306 -- test fake must be executable

		outDir := filepath.Join(root, "out")
		pkgs := map[string]string{"proto/foo": modeStock}
		require.NoError(t, generateNetwork(map[string]string{}, outDir, binDir, t.TempDir(), pkgs))

		got, readErr := os.ReadFile(filepath.Join(outDir, "proto", "foo", "foo.pb.go"))
		require.NoError(t, readErr)
		require.Equal(t, "package foo\n", string(got))
	})
}

func TestCompileDescriptors(t *testing.T) {
	stage := t.TempDir()
	write := func(name, content string) {
		require.NoError(t, os.WriteFile(filepath.Join(stage, name), []byte(content), 0o600))
	}
	// a.proto and b.proto both import common.proto. Visiting b after a finds
	// common already seen, exercising the dedup early-return.
	write("common.proto", "syntax = \"proto3\";\npackage common;\nmessage C { int32 x = 1; }\n")
	write("a.proto", "syntax = \"proto3\";\npackage a;\nimport \"common.proto\";\nmessage A { common.C c = 1; }\n")
	write("b.proto", "syntax = \"proto3\";\npackage b;\nimport \"common.proto\";\nmessage B { common.C c = 1; }\n")

	got, err := compileDescriptors(stage, t.TempDir(), []string{"a.proto", "b.proto"})
	require.NoError(t, err)

	// common.proto is shared but must appear exactly once thanks to the dedup.
	names := make(map[string]int)
	for _, fd := range got {
		names[fd.GetName()]++
	}
	require.Equal(t, 1, names["common.proto"])
	require.Equal(t, 1, names["a.proto"])
	require.Equal(t, 1, names["b.proto"])
	require.Equal(t, 3, len(got))
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	require.NoError(t, os.WriteFile(src, []byte("hello\n"), 0o600))

	require.NoError(t, copyFile(src, dst))

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, "hello\n", string(got))
}

func TestWriteTagged(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	require.NoError(t, os.WriteFile(src, []byte("package foo\n"), 0o600))

	require.NoError(t, writeTagged("!minimal", src, dst))

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, "//go:build !minimal\n\npackage foo\n", string(got))
}

func TestPluginForMode(t *testing.T) {
	const binDir = "/tmp/bin"
	castPlugin := filepath.Join(binDir, "protoc-gen-go-cast")
	// stockPlugin := filepath.Join(binDir, "protoc-gen-go")

	t.Run("cast uses the cast plugin with the base opt", func(t *testing.T) {
		plugin, param, err := pluginForMode(modeCast, binDir, "base")
		require.NoError(t, err)
		require.Equal(t, castPlugin, plugin)
		require.Equal(t, "base", param)
	})

	t.Run("cast_grpc uses the cast plugin and appends the grpc param", func(t *testing.T) {
		plugin, param, err := pluginForMode(modeCastGRPC, binDir, "base")
		require.NoError(t, err)
		require.Equal(t, castPlugin, plugin)
		require.Equal(t, "base,plugins=grpc", param)
	})
}

func TestStageProtos(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "proto", "eth"), 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "proto", "eth", "types.proto"),
		[]byte("// FOO and F\nAA BB\n"),
		0o600,
	))
	// A non-.proto sibling must not be staged.
	require.NoError(t, os.WriteFile(filepath.Join(root, "proto", "eth", "notes.txt"), []byte("FOO"), 0o600))
	t.Chdir(root)

	// Keys are applied longest-first: "FOO" -> "Bar" before "F" -> "X", so the
	// "FOO" token does not get clobbered by the shorter "F" substitution. The
	// equal-length "AA"/"BB" keys force the comparator's lexical tie-breaker.
	dict := map[string]string{"FOO": "Bar", "F": "X", "AA": "1", "BB": "2"}
	stage := filepath.Join(root, "stage")
	require.NoError(t, stageProtos(stage, dict))

	got, err := os.ReadFile(filepath.Join(stage, "proto", "eth", "types.proto"))
	require.NoError(t, err)
	require.Equal(t, "// Bar and X\n1 2\n", string(got))

	// The non-proto file was skipped.
	_, statErr := os.Stat(filepath.Join(stage, "proto", "eth", "notes.txt"))
	require.Equal(t, true, os.IsNotExist(statErr))
}

func TestPkgProtos(t *testing.T) {
	t.Run("returns the sorted .proto files in the directory", func(t *testing.T) {
		dir := t.TempDir()
		write := func(name string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600))
		}
		// Out of order, with non-.proto siblings and a subdirectory to ignore.
		write("zeta.proto")
		write("alpha.proto")
		write("notes.txt")
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "nested.proto"), 0o700))

		got, err := pkgProtos(dir)
		require.NoError(t, err)
		require.DeepEqual(t, []string{
			filepath.ToSlash(filepath.Join(dir, "alpha.proto")),
			filepath.ToSlash(filepath.Join(dir, "zeta.proto")),
		}, got)
	})

	t.Run("errors when the directory cannot be read", func(t *testing.T) {
		got, err := pkgProtos(filepath.Join(t.TempDir(), "does-not-exist"))
		require.IsNil(t, got)
		require.ErrorContains(t, "readDir", err)
	})
}
