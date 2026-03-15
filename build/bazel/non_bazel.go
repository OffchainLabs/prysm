package bazel

import (
	"os"
	"path/filepath"
)

// BuiltWithBazel returns true iff this library was built with Bazel.
func BuiltWithBazel() bool {
	return false
}

// FindBinary looks for a binary in PATH.
func FindBinary(pkg, name string) (string, bool) {
	p, err := findBinaryInPath(name)
	if err != nil {
		return "", false
	}
	return p, true
}

func findBinaryInPath(name string) (string, error) {
	return filepath.Abs(name)
}

// Runfile returns the absolute path to the given file, assuming it is
// relative to the workspace root.
func Runfile(path string) (string, error) {
	return filepath.Abs(path)
}

// RunfilesPath returns the current working directory.
func RunfilesPath() (string, error) {
	return os.Getwd()
}

// TestTmpDir returns a temporary directory for tests.
func TestTmpDir() string {
	return os.TempDir()
}

// NewTmpDir creates a new temporary directory with the given prefix.
func NewTmpDir(prefix string) (string, error) {
	return os.MkdirTemp("", prefix)
}

// RelativeTestTargetPath returns an empty string outside of Bazel.
func RelativeTestTargetPath() string {
	return ""
}

// SetGoEnv is a no-op outside of Bazel.
func SetGoEnv() {}
