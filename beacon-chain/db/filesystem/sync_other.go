//go:build !linux

package filesystem

// syncFilesystem is a no-op: there is no portable equivalent of Linux syncfs.
// Data written shortly before a host crash may then be missing while the cache snapshot says it is stored.
func syncFilesystem(string) error {
	return nil
}
