//go:build !windows

package filesystem

import "os"

func renameProtectionFile(from, to string) error {
	return os.Rename(from, to)
}
