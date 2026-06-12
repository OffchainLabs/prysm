// Copyright 2015 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

//go:build !bazel

// Non-Bazel implementations of the runfiles helpers. When built with the Go
// toolchain (no `bazel` build tag), files are resolved from the source tree and
// from a local test-data cache instead of a Bazel runfiles directory.

package bazel

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// BuiltWithBazel returns true iff this library was built with Bazel.
func BuiltWithBazel() bool {
	return false
}

// moduleRoot walks up from the current working directory to find the module
// root (the directory containing go.mod). `go test` runs with the working
// directory set to the package under test, so this reliably finds the repo root.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not locate go.mod above %q", dir)
		}

		dir = parent
	}
}

// testdataRoot returns the directory holding downloaded external test data:
// <module root>/third_party/testdata. Populated lazily by build/externaldata (and
// eagerly by `make testdata`).
func testdataRoot() string {
	if root, err := moduleRoot(); err == nil {
		return filepath.Join(root, "third_party", "testdata")
	}

	return ""
}

// searchBases is the ordered list of directories Runfile/ListRunfiles resolve
// relative paths against.
func searchBases() []string {
	bases := []string{}
	if cwd, err := os.Getwd(); err == nil {
		bases = append(bases, cwd)
	}

	if root, err := moduleRoot(); err == nil {
		bases = append(bases, root)
	}

	if td := testdataRoot(); td != "" {
		bases = append(bases, td)
	}

	return bases
}

// Runfile resolves path against, in order: an absolute path, the current working
// directory, the module root, and the test-data root. External test data is a
// lazy fixture: if the path is not on disk but maps to a known external-data
// archive, that archive is fetched (once, idempotently) and resolution retried.
// It returns an error if the file/dir still cannot be found.
func Runfile(path string) (string, error) {
	if filepath.IsAbs(path) {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}

		return "", fmt.Errorf("runfile not found: %s", path)
	}

	if resolved, ok := findRunfile(path); ok {
		return resolved, nil
	}

	if fetchOnMiss(path) {
		if resolved, ok := findRunfile(path); ok {
			return resolved, nil
		}
	}

	return "", fmt.Errorf("Runfile %s: could not locate file (external test data is fetched on demand; check network access)", path)
}

// findRunfile searches the resolution bases for a (relative) path and returns the
// first existing candidate.
func findRunfile(path string) (string, bool) {
	for _, base := range searchBases() {
		candidate := filepath.Join(base, path)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
	}

	return "", false
}

// RunfilesPath returns the module root.
func RunfilesPath() (string, error) {
	return moduleRoot()
}

// ListRunfiles walks the test-data root and returns every file as a RunfileEntry.
// ShortPath is the slash-separated path relative to that root, which callers
// filter on (e.g. "eip4881_spec_tests", "generated/").
func ListRunfiles() ([]RunfileEntry, error) {
	root := testdataRoot()
	if root == "" {
		return nil, fmt.Errorf("could not locate test-data root")
	}

	if _, err := os.Stat(root); err != nil {
		return nil, fmt.Errorf("test-data root %q not found (run `make testdata`): %w", root, err)
	}

	var entries []RunfileEntry
	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(root, p)
		if err != nil {
			return fmt.Errorf("rel: %w", err)
		}

		entries = append(entries, RunfileEntry{Path: p, ShortPath: filepath.ToSlash(rel)})
		return nil
	})

	if walkErr != nil {
		return nil, walkErr
	}

	return entries, nil
}

// TestTmpDir returns the OS temporary directory.
func TestTmpDir() string {
	return os.TempDir()
}

// NewTmpDir creates a new temporary directory with the given prefix. The caller
// is responsible for cleaning it up.
func NewTmpDir(prefix string) (string, error) {
	return os.MkdirTemp("", prefix)
}

// RelativeTestTargetPath has no meaning outside Bazel.
func RelativeTestTargetPath() string {
	return ""
}

// SetGoEnv is a no-op outside Bazel: the Go toolchain is already on PATH.
func SetGoEnv() {}
