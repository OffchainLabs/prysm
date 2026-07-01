package externaldata

import (
	"fmt"
	"os"
	"path/filepath"
)

// Root returns the directory holding downloaded external test data:
// <module root>/third_party/testdata. This mirrors build/bazel.testdataRoot (kept
// independent to avoid an import cycle: build/bazel imports this package).
func Root() string {
	if root, err := moduleRoot(); err == nil {
		return filepath.Join(root, "third_party", "testdata")
	}

	return ""
}

// moduleRoot walks up from the working directory to the directory containing
// go.mod. `go test` runs with the working directory in the package under test,
// so this reliably finds the repo root.
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
			return "", os.ErrNotExist
		}

		dir = parent
	}
}
