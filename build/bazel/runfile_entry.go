package bazel

// RunfileEntry describes a single runfile.
type RunfileEntry struct {
	// Path is the absolute path to the file on disk.
	Path string

	// ShortPath is the path relative to the runfiles (or test-data) root, using
	// forward slashes.
	ShortPath string

	// Workspace is the workspace/repository the file originated from.
	// (It is left empty by the non-Bazel implementation.)
	Workspace string
}
