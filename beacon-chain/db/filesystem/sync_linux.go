//go:build linux

package filesystem

import (
	"os"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// syncFilesystem flushes to disk all the pending writes of the filesystem containing `path`.
func syncFilesystem(path string) (err error) {
	file, err := os.Open(path) // #nosec G304 -- path is the data column storage base directory
	if err != nil {
		return errors.Wrap(err, "open")
	}

	defer func() {
		closeErr := file.Close()

		// Overwrite the existing error only if it is nil, since the close error is less important.
		if closeErr != nil && err == nil {
			err = errors.Wrap(closeErr, "close")
		}
	}()

	return unix.Syncfs(int(file.Fd()))
}
