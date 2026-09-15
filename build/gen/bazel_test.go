package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/bazelbuild/buildtools/build"
)

// parseContent parses Bazel source into a *build.File, failing the test on error.
func parseContent(t *testing.T, content string) *build.File {
	t.Helper()
	f, err := build.Parse("test.bzl", []byte(content))
	require.NoError(t, err)

	return f
}

func TestParseBazel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "BUILD.bazel")
	content := `go_proto_library(
    name = "go_default_library",
    compilers = ["//proto:cast_compiler"],
)
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	f, err := parseBazel(path)
	require.NoError(t, err)
	require.NotNil(t, f)
	require.Equal(t, path, f.Path)

	rules := f.Rules("go_proto_library")
	require.Equal(t, 1, len(rules))
	require.Equal(t, "go_default_library", rules[0].Name())
}

func TestTopLevelAssignments(t *testing.T) {
	t.Run("collects assignments of different expression kinds", func(t *testing.T) {
		env := topLevelAssignments(parseContent(t, `
name = "value"
items = ["a", "b"]
mapping = {"k": "v"}
`))

		require.Equal(t, 3, len(env))

		str, ok := env["name"].(*build.StringExpr)
		require.Equal(t, true, ok)
		require.Equal(t, "value", str.Value)

		list, ok := env["items"].(*build.ListExpr)
		require.Equal(t, true, ok)
		require.Equal(t, 2, len(list.List))

		_, ok = env["mapping"].(*build.DictExpr)
		require.Equal(t, true, ok)
	})

	t.Run("ignores augmented assignments", func(t *testing.T) {
		env := topLevelAssignments(parseContent(t, `
	x = ["a"]
	x += ["b"]
	`))

		// The "+=" statement is skipped, so only the original "=" binding remains.
		list, ok := env["x"].(*build.ListExpr)
		require.Equal(t, true, ok)
		require.Equal(t, 1, len(list.List))
	})
}

func TestLoadSSZDicts(t *testing.T) {
	// writeSSZBzl creates proto/ssz_proto_library.bzl under a temp dir and
	// chdirs into it, so loadSSZDicts resolves its fixed relative path there.
	writeSSZBzl := func(t *testing.T, content string) {
		t.Helper()
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "proto"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, sszProtoLibraryBzl), []byte(content), 0o600))
		t.Chdir(dir)
	}

	writeSSZBzl(t, `
mainnet = {
    "Foo": "1",
    "Bar": "2",
}
minimal = {
    "Foo": "3",
}
`)

	mainnet, minimal, err := loadSSZDicts()
	require.NoError(t, err)
	require.DeepEqual(t, map[string]string{"Foo": "1", "Bar": "2"}, mainnet)
	require.DeepEqual(t, map[string]string{"Foo": "3"}, minimal)

}

func TestStringDict(t *testing.T) {
	// envOf parses top-level assignments so the named dict can be looked up.
	envOf := func(t *testing.T, src string) map[string]build.Expr {
		t.Helper()
		return topLevelAssignments(parseContent(t, src))
	}

	t.Run("resolves a dict to a string map", func(t *testing.T) {
		env := envOf(t, `d = {"Foo": "1", "Bar": "2"}`)
		got, err := stringDict(env, "d")
		require.NoError(t, err)
		require.DeepEqual(t, map[string]string{"Foo": "1", "Bar": "2"}, got)
	})

	t.Run("errors when the name is absent", func(t *testing.T) {
		got, err := stringDict(envOf(t, `d = {}`), "missing")
		require.IsNil(t, got)
		require.ErrorContains(t, `dict "missing" not found`, err)
	})

	t.Run("errors when the value is not a dict", func(t *testing.T) {
		env := envOf(t, `d = "not a dict"`)
		got, err := stringDict(env, "d")
		require.IsNil(t, got)
		require.ErrorContains(t, `"d" is not a dict`, err)
	})

	t.Run("errors on a non-string key", func(t *testing.T) {
		env := envOf(t, `d = {1: "a"}`)
		got, err := stringDict(env, "d")
		require.IsNil(t, got)
		require.ErrorContains(t, `"d" has a non-string key`, err)
	})

	t.Run("errors on a non-string value", func(t *testing.T) {
		env := envOf(t, `d = {"Foo": 1}`)
		got, err := stringDict(env, "d")
		require.IsNil(t, got)
		require.ErrorContains(t, "non-string value", err)
	})
}

func TestBuildBazelFiles(t *testing.T) {
	// writeFile creates a file (and parent dirs) relative to dir.
	writeFile := func(t *testing.T, dir, rel string) {
		t.Helper()
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("# test\n"), 0o600))
	}

	dir := t.TempDir()
	// Created out of order, with non-matching siblings interleaved.
	writeFile(t, dir, "proto/zeta/BUILD.bazel")
	writeFile(t, dir, "proto/alpha/BUILD.bazel")
	writeFile(t, dir, "proto/BUILD.bazel")
	writeFile(t, dir, "proto/alpha/notes.txt")
	writeFile(t, dir, "proto/beta/BUILD") // no .bazel suffix
	t.Chdir(dir)

	got, err := buildBazelFiles()
	require.NoError(t, err)
	require.DeepEqual(t, []string{
		"proto/BUILD.bazel",
		"proto/alpha/BUILD.bazel",
		"proto/zeta/BUILD.bazel",
	}, got)

}

func TestLoadProtoPkgs(t *testing.T) {
	// writeBuild writes proto/<pkg>/BUILD.bazel with the given content under dir.
	writeBuild := func(t *testing.T, dir, pkg, content string) {
		t.Helper()
		path := filepath.Join(dir, "proto", filepath.FromSlash(pkg), "BUILD.bazel")
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}

	dir := t.TempDir()
	writeBuild(t, dir, "grpc", `go_proto_library(name = "go_default_library", compilers = ["//:cast_grpc_compiler"])`)
	writeBuild(t, dir, "cast", `go_proto_library(name = "go_default_library", compilers = ["//:cast_compiler"])`)
	writeBuild(t, dir, "stock", `go_proto_library(name = "go_default_library", compilers = ["//:go_proto_compiler"])`)
	// No go_proto_library rule -> not included in the result.
	writeBuild(t, dir, "lib", `go_library(name = "go_default_library")`)
	t.Chdir(dir)

	got, err := loadProtoPkgs()
	require.NoError(t, err)
	require.DeepEqual(t, map[string]string{
		"proto/grpc":  modeCastGRPC,
		"proto/cast":  modeCast,
		"proto/stock": modeStock,
	}, got)
}

func TestCompilerMode(t *testing.T) {
	rules := parseContent(t, `go_proto_library(name = "x", compiler = "//:cast_grpc_compiler")`).Rules("go_proto_library")
	require.Equal(t, 1, len(rules))

	require.Equal(t, modeCastGRPC, compilerMode(rules[0]))
}

func TestLoadMethodicalTargets(t *testing.T) {
	// writeBuild writes proto/<pkg>/BUILD.bazel with the given content under dir.
	writeBuild := func(t *testing.T, dir, pkg, content string) {
		t.Helper()
		path := filepath.Join(dir, "proto", filepath.FromSlash(pkg), "BUILD.bazel")
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}

	dir := t.TempDir()
	// "bbb" is written first but must sort after "aaa".
	writeBuild(t, dir, "bbb", `
ssz_methodical(
    name = "methodical_b",
    deps = [":go_proto"],
    config_file = "b.yaml",
    out = "b.ssz.go",
)
`)
	// "aaa" holds two rules; both must appear, in file order. The second omits
	// override_package_name to exercise the optional attr.
	writeBuild(t, dir, "aaa", `
ssz_methodical(
    name = "methodical_a1",
    deps = [":go_proto"],
    config_file = "a1.yaml",
    override_package_name = "eth",
    out = "a1.ssz.go",
)
ssz_methodical(
    name = "methodical_a2",
    config_file = "a2.yaml",
    out = "a2.ssz.go",
)
`)
	// No ssz_methodical rule -> skipped entirely.
	writeBuild(t, dir, "ccc", `go_library(name = "go_default_library")`)
	t.Chdir(dir)

	got, err := loadMethodicalTargets()
	require.NoError(t, err)
	require.DeepEqual(t, []methodicalTarget{
		{
			pkg:         "proto/aaa",
			configFile:  "a1.yaml",
			out:         "a1.ssz.go",
			overridePkg: "eth",
		},
		{
			pkg:        "proto/aaa",
			configFile: "a2.yaml",
			out:        "a2.ssz.go",
		},
		{
			pkg:        "proto/bbb",
			configFile: "b.yaml",
			out:        "b.ssz.go",
		},
	}, got)
}
