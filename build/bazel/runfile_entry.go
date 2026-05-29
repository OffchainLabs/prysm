package bazel

// RunfileEntry describes a single runfile, mirroring the shape returned by
// rules_go's bazel.ListRunfiles so callers can switch between the Bazel and
// non-Bazel implementations without changing field access.
type RunfileEntry struct {
	// Path is the absolute path to the file on disk.
	Path string
	// ShortPath is the path relative to the runfiles (or test-data) root, using
	// forward slashes. Callers filter on substrings of this (e.g. the external
	// repository name).
	ShortPath string
	// Workspace is the workspace/repository the file originated from. It is left
	// empty by the non-Bazel implementation.
	Workspace string
}
