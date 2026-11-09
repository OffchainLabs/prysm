//go:build !bazel

package main

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/bazelbuild/buildtools/build"
)

// TestAnalyzersMatchNogo checks that prysm-vet runs the same analyzers as the `nogo` rule of the
// root BUILD.bazel. It does not run under Bazel (no access to the root BUILD.bazel). Remove it
// along with Bazel.
func TestAnalyzersMatchNogo(t *testing.T) {
	data, err := os.ReadFile("../../BUILD.bazel")
	require.NoError(t, err)

	f, err := build.ParseBuild("BUILD.bazel", data)
	require.NoError(t, err)

	// Analyzer packages of the nogo rule, skipping the select() keys and the staticcheck
	// template (the staticcheck checks come from STATICCHECK_ANALYZERS).
	var nogo *build.Rule
	for _, r := range f.Rules("nogo") {
		if r.Name() == "nogo" {
			nogo = r
		}
	}
	require.NotNil(t, nogo, "nogo rule not found")

	var bazelPkgs []string
	build.Walk(nogo.Attr("deps"), func(e build.Expr, _ []build.Expr) {
		if s, ok := e.(*build.StringExpr); ok && strings.HasSuffix(s.Value, ":go_default_library") && !strings.Contains(s.Value, "%s") {
			bazelPkgs = append(bazelPkgs, importPath(t, s.Value))
		}
	})

	// Analyzer packages imported by analyzers.go.
	src, err := parser.ParseFile(token.NewFileSet(), "analyzers.go", nil, parser.ImportsOnly)
	require.NoError(t, err)

	var goPkgs []string
	for _, imp := range src.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		require.NoError(t, err)

		if strings.HasPrefix(path, "github.com/OffchainLabs/prysm/v7/tools/analyzers/") ||
			strings.HasPrefix(path, "golang.org/x/tools/go/analysis/passes/") {
			goPkgs = append(goPkgs, path)
		}
	}

	slices.Sort(bazelPkgs)
	slices.Sort(goPkgs)
	require.DeepEqual(t, bazelPkgs, goPkgs)

	// Staticcheck checks.
	var bazelChecks []string
	for _, stmt := range f.Stmt {
		assign, ok := stmt.(*build.AssignExpr)
		if !ok {
			continue
		}

		if lhs, ok := assign.LHS.(*build.Ident); !ok || lhs.Name != "STATICCHECK_ANALYZERS" {
			continue
		}

		list, ok := assign.RHS.(*build.ListExpr)
		require.Equal(t, true, ok, "STATICCHECK_ANALYZERS is not a list")

		for _, e := range list.List {
			bazelChecks = append(bazelChecks, e.(*build.StringExpr).Value)
		}
	}

	require.DeepEqual(t, bazelChecks, staticcheckChecks)
}

// importPath maps a nogo dependency label to its Go import path.
func importPath(t *testing.T, label string) string {
	pkg, _, _ := strings.Cut(label, ":")
	switch {
	case strings.HasPrefix(pkg, "//"):
		return "github.com/OffchainLabs/prysm/v7/" + strings.TrimPrefix(pkg, "//")
	case strings.HasPrefix(pkg, "@org_golang_x_tools//"):
		return "golang.org/x/tools/" + strings.TrimPrefix(pkg, "@org_golang_x_tools//")
	}

	t.Fatalf("unexpected nogo dependency %q", label)
	return ""
}

// TestLintPassesMatchBazel checks that the Makefile's develop and minimal lint passes list the
// packages of the Bazel targets built in those configurations. Remove it along with Bazel.
func TestLintPassesMatchBazel(t *testing.T) {
	const root = "../.."

	var develop, minimal []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			switch d.Name() {
			case ".git", "testdata", "third_party", "node_modules":
				return filepath.SkipDir
			}

			if strings.HasPrefix(d.Name(), "bazel-") {
				return filepath.SkipDir
			}

			return nil
		}

		if d.Name() != "BUILD.bazel" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		f, err := build.ParseBuild(path, data)
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}

		pkg := "./" + filepath.ToSlash(rel)
		for _, r := range f.Rules("") {
			if r.AttrString("eth_network") == "minimal" && !slices.Contains(minimal, pkg) {
				minimal = append(minimal, pkg)
			}

			if slices.Contains(r.AttrStrings("gotags"), "develop") && !slices.Contains(develop, pkg) {
				develop = append(develop, pkg)
			}
		}

		return nil
	})
	require.NoError(t, err)

	slices.Sort(develop)
	slices.Sort(minimal)

	vars := makefileVars(t, filepath.Join(root, "Makefile"))
	require.DeepEqual(t, develop, vars["VET_DEVELOP"])
	require.DeepEqual(t, minimal, vars["VET_MINIMAL"])
}

// makefileVars returns the `NAME := values...` assignments of a Makefile, joining continued lines.
func makefileVars(t *testing.T, path string) map[string][]string {
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	vars := make(map[string][]string)
	text := strings.ReplaceAll(string(data), "\\\n", " ")
	for line := range strings.SplitSeq(text, "\n") {
		name, value, ok := strings.Cut(line, ":=")
		name = strings.TrimSpace(name)
		if !ok || name == "" || strings.ContainsAny(name, " \t#") {
			continue
		}

		vars[name] = strings.Fields(value)
	}

	return vars
}
