package main

// This file is the single point where build/gen reads its generation config
// from the Bazel files, which are the source of truth:
//
//   - proto/ssz_proto_library.bzl  -> the mainnet/minimal SSZ substitution dicts
//   - proto/**/BUILD.bazel         -> the proto package list + plugin mode
//                                     (go_proto_library) and the SSZ targets
//                                     (ssz_methodical)
//
// Nothing else in build/gen hardcodes this config. When Bazel is eventually
// removed, only this file needs to change.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bazelbuild/buildtools/build"
)

const sszProtoLibraryBzl = "proto/ssz_proto_library.bzl"

// parseBazel reads and parses a BUILD.bazel or .bzl file.
func parseBazel(path string) (*build.File, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is a repo-relative Bazel file
	if err != nil {
		return nil, fmt.Errorf("readFile: %w", err)
	}

	f, err := build.Parse(path, data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	return f, nil
}

// topLevelAssignments collects the file's top-level `name = <expr>` statements.
func topLevelAssignments(f *build.File) map[string]build.Expr {
	env := make(map[string]build.Expr)
	for _, stmt := range f.Stmt {
		assign, ok := stmt.(*build.AssignExpr)
		if !ok || assign.Op != "=" {
			continue
		}

		if lhs, ok := assign.LHS.(*build.Ident); ok {
			env[lhs.Name] = assign.RHS
		}
	}

	return env
}

// loadSSZDicts returns the mainnet and minimal SSZ-size substitution maps.
func loadSSZDicts() (mainnet, minimal map[string]string, err error) {
	f, err := parseBazel(sszProtoLibraryBzl)
	if err != nil {
		return nil, nil, err
	}

	env := topLevelAssignments(f)

	mainnet, err = stringDict(env, "mainnet")
	if err != nil {
		return nil, nil, fmt.Errorf("string dict: %w", err)
	}

	minimal, err = stringDict(env, "minimal")
	if err != nil {
		return nil, nil, fmt.Errorf("string dict: %w", err)
	}

	return mainnet, minimal, nil
}

// stringDict resolves a top-level `name = {...}` assignment to a string map.
func stringDict(env map[string]build.Expr, name string) (map[string]string, error) {
	e, ok := env[name]
	if !ok {
		return nil, fmt.Errorf("%s: dict %q not found", sszProtoLibraryBzl, name)
	}

	dict, ok := e.(*build.DictExpr)
	if !ok {
		return nil, fmt.Errorf("%s: %q is not a dict (%T)", sszProtoLibraryBzl, name, e)
	}

	out := make(map[string]string, len(dict.List))
	for _, kv := range dict.List {
		key, ok := kv.Key.(*build.StringExpr)
		if !ok {
			return nil, fmt.Errorf("%s: %q has a non-string key", sszProtoLibraryBzl, name)
		}

		val, ok := kv.Value.(*build.StringExpr)
		if !ok {
			return nil, fmt.Errorf("%s: %q[%q] has a non-string value", sszProtoLibraryBzl, name, key.Value)
		}

		out[key.Value] = val.Value
	}

	return out, nil
}

// buildBazelFiles returns every proto/**/BUILD.bazel path, sorted.
func buildBazelFiles() ([]string, error) {
	var paths []string
	err := filepath.WalkDir("proto", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk dir: %w", err)
		}

		if !d.IsDir() && d.Name() == "BUILD.bazel" {
			paths = append(paths, filepath.ToSlash(path))
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walkDir: %w", err)
	}

	sort.Strings(paths)

	return paths, nil
}

// loadProtoPkgs returns the proto package directories that define a
// go_proto_library, mapped to their protoc plugin mode (cast / cast_grpc /
// stock).
func loadProtoPkgs() (map[string]string, error) {
	files, err := buildBazelFiles()
	if err != nil {
		return nil, fmt.Errorf("build bazel files: %w", err)
	}

	pkgs := make(map[string]string)
	for _, path := range files {
		f, err := parseBazel(path)
		if err != nil {
			return nil, fmt.Errorf("parse bazel: %w", err)
		}

		rules := f.Rules("go_proto_library")
		if len(rules) == 0 {
			continue
		}

		dir := filepath.ToSlash(filepath.Dir(path))
		pkgs[dir] = compilerMode(rules[0])
	}

	return pkgs, nil
}

// compilerMode maps a go_proto_library rule's compiler label(s) to a protoc
// plugin mode.
func compilerMode(r *build.Rule) string {
	labels := r.AttrStrings("compilers")
	if s := r.AttrString("compiler"); s != "" {
		labels = append(labels, s)
	}

	for _, l := range labels {
		if strings.Contains(l, "grpc") {
			return modeCastGRPC
		}
	}

	for _, l := range labels {
		if strings.Contains(l, "cast") {
			return modeCast
		}
	}

	return modeStock
}

// methodicalTarget describes one ssz_methodical rule: the config file and
// output file (both relative to the proto package dir) plus an optional
// generated-package-name override.
type methodicalTarget struct {
	pkg         string // proto package dir, e.g. "proto/prysm/v1alpha1"
	configFile  string // config_file attr, relative to pkg
	out         string // out attr, relative to pkg
	overridePkg string // override_package_name attr, "" if unset
}

// loadMethodicalTargets returns the SSZ generation targets read from the
// ssz_methodical rules across proto/**/BUILD.bazel, in (sorted path, file)
// order.
func loadMethodicalTargets() ([]methodicalTarget, error) {
	files, err := buildBazelFiles()
	if err != nil {
		return nil, fmt.Errorf("build bazel files: %w", err)
	}

	var targets []methodicalTarget
	for _, path := range files {
		f, err := parseBazel(path)
		if err != nil {
			return nil, fmt.Errorf("parse bazel: %w", err)
		}

		rules := f.Rules("ssz_methodical")
		if len(rules) == 0 {
			continue
		}

		pkg := filepath.ToSlash(filepath.Dir(path))
		for _, r := range rules {
			cfg := r.AttrString("config_file")
			if cfg == "" {
				return nil, fmt.Errorf("%s: %s: missing config_file", path, r.Name())
			}

			out := r.AttrString("out")
			if out == "" {
				return nil, fmt.Errorf("%s: %s: missing out", path, r.Name())
			}

			targets = append(targets, methodicalTarget{
				pkg:         pkg,
				configFile:  cfg,
				out:         out,
				overridePkg: r.AttrString("override_package_name"),
			})
		}
	}

	return targets, nil
}
