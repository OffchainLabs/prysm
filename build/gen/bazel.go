package main

// This file is the single point where build/gen reads its generation config
// from the Bazel files, which are the source of truth:
//
//   - proto/ssz_proto_library.bzl  -> the mainnet/minimal SSZ substitution dicts
//   - proto/**/BUILD.bazel         -> the proto package list + plugin mode
//                                     (go_proto_library) and the SSZ targets
//                                     (ssz_gen_marshal)
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

// evalStringList resolves an expression to a flat slice of strings.
func evalStringList(e build.Expr, env map[string]build.Expr) ([]string, error) {
	switch v := e.(type) {
	case *build.StringExpr:
		return []string{v.Value}, nil
	case *build.ListExpr:
		var out []string
		for _, item := range v.List {
			sub, err := evalStringList(item, env)
			if err != nil {
				return nil, err
			}

			out = append(out, sub...)
		}

		return out, nil
	case *build.Ident:
		ref, ok := env[v.Name]
		if !ok {
			return nil, fmt.Errorf("unresolved identifier %q", v.Name)
		}

		return evalStringList(ref, env)
	case *build.BinaryExpr:
		if v.Op != "+" {
			return nil, fmt.Errorf("unsupported binary op %q", v.Op)
		}

		left, err := evalStringList(v.X, env)
		if err != nil {
			return nil, err
		}

		right, err := evalStringList(v.Y, env)
		if err != nil {
			return nil, err
		}

		return append(left, right...), nil
	default:
		return nil, fmt.Errorf("unsupported expression %T", e)
	}
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
		return nil, nil, err
	}

	minimal, err = stringDict(env, "minimal")
	if err != nil {
		return nil, nil, err
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
			return err
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
		return nil, err
	}

	pkgs := make(map[string]string)
	for _, path := range files {
		f, err := parseBazel(path)
		if err != nil {
			return nil, err
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

// loadSSZTargets returns the SSZ generation targets read from the
// ssz_gen_marshal rules across proto/**/BUILD.bazel, in (sorted path, file)
// order.
func loadSSZTargets() ([]sszTarget, error) {
	files, err := buildBazelFiles()
	if err != nil {
		return nil, err
	}

	var targets []sszTarget
	for _, path := range files {
		f, err := parseBazel(path)
		if err != nil {
			return nil, err
		}

		rules := f.Rules("ssz_gen_marshal")
		if len(rules) == 0 {
			continue
		}

		env := topLevelAssignments(f)
		pkg := filepath.ToSlash(filepath.Dir(path))
		for _, r := range rules {
			t, err := sszTargetFromRule(r, env, pkg)
			if err != nil {
				return nil, fmt.Errorf("%s: %s: %w", path, r.Name(), err)
			}

			targets = append(targets, t)
		}
	}

	return targets, nil
}

// sszTargetFromRule converts a single ssz_gen_marshal rule into an sszTarget.
func sszTargetFromRule(r *build.Rule, env map[string]build.Expr, pkg string) (sszTarget, error) {
	out := r.AttrString("out")
	if out == "" {
		return sszTarget{}, fmt.Errorf("missing out")
	}

	objs, err := attrStringList(r, env, "objs")
	if err != nil {
		return sszTarget{}, err
	}

	exclude, err := attrStringList(r, env, "exclude_objs")
	if err != nil {
		return sszTarget{}, err
	}

	includes, err := attrStringList(r, env, "includes")
	if err != nil {
		return sszTarget{}, err
	}

	libInc, protoInc := splitIncludes(includes)

	return sszTarget{
		pkg:      pkg,
		out:      out,
		libInc:   libInc,
		protoInc: protoInc,
		objs:     objs,
		exclude:  exclude,
	}, nil
}

// attrStringList resolves a rule attribute to a string slice, or nil if absent.
func attrStringList(r *build.Rule, env map[string]build.Expr, key string) ([]string, error) {
	e := r.Attr(key)
	if e == nil {
		return nil, nil
	}

	return evalStringList(e, env)
}

// splitIncludes maps Bazel include labels (e.g. "//math:go_default_library") to
// the two buckets the sszgen invocation expects: proto packages (whose .pb.go
// must be staged) and plain Go library import paths.
func splitIncludes(labels []string) (libInc, protoInc []string) {
	for _, l := range labels {
		path := strings.TrimPrefix(l, "//")
		if i := strings.IndexByte(path, ':'); i >= 0 {
			path = path[:i]
		}

		if strings.HasPrefix(path, "proto/") {
			protoInc = append(protoInc, path)
		} else {
			libInc = append(libInc, path)
		}
	}

	return libInc, protoInc
}
