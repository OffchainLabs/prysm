package main

import (
	"fmt"
	"go/token"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// install makes the analyzers behave as under nogo, in place: config flags are set, file
// versions are hidden, and diagnostics silenced by a nolint comment or filtered out by the config
// are dropped.
// Analyzers are modified rather than copied so that those also required by others (buildssa,
// inspect...) keep a single identity and are not run twice.
func install(analyzers []*analysis.Analyzer, configs map[string]*config, root string) error {
	var nolint nolintCache
	for _, a := range analyzers {
		cfg := configFor(configs, a.Name)
		if cfg != nil {
			for flag, value := range cfg.analyzerFlags {
				if err := a.Flags.Set(strings.TrimLeft(flag, "-"), value); err != nil {
					return fmt.Errorf("%s: set flag %q: %w", a.Name, flag, err)
				}
			}
		}

		run, name := a.Run, a.Name
		a.Run = func(pass *analysis.Pass) (any, error) {
			// nogo's type checker records no file versions, which turns off the analyzers gated
			// on a Go version (most modernize ones). Do the same on a copy, as TypesInfo is shared
			// by the analyzers of the package.
			info := *pass.TypesInfo
			info.FileVersions = nil
			pass.TypesInfo = &info

			report := pass.Report
			pass.Report = func(d analysis.Diagnostic) {
				if nolint.silenced(pass.Fset, pass.Files, name, d.Pos) {
					return
				}

				if cfg != nil && !cfg.keep(relativeFilename(pass.Fset, d.Pos, root)) {
					return
				}

				report(d)
			}

			return run(pass)
		}
	}

	return nil
}

// relativeFilename returns the file holding pos, relative to root when possible (as nogo does
// relative to the Bazel exec root). Diagnostics without a valid position map to "-".
func relativeFilename(fset *token.FileSet, pos token.Pos, root string) string {
	p := fset.Position(pos)
	if !p.IsValid() {
		return "-"
	}

	if root == "" {
		return p.Filename
	}

	rel, err := filepath.Rel(root, p.Filename)
	if err != nil {
		return p.Filename
	}

	return filepath.ToSlash(rel)
}
