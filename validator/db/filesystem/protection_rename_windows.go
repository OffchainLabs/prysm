package filesystem

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func renameProtectionFile(from, to string) error {
	from, err := protectionWindowsPath(from)
	if err != nil {
		return fmt.Errorf("resolve protection source path: %w", err)
	}
	to, err = protectionWindowsPath(to)
	if err != nil {
		return fmt.Errorf("resolve protection destination path: %w", err)
	}
	source, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return fmt.Errorf("encode protection source path %s: %w", from, err)
	}
	destination, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return fmt.Errorf("encode protection destination path %s: %w", to, err)
	}
	if err := windows.MoveFileEx(source, destination, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return fmt.Errorf("replace protection file %s with %s: %w", to, from, err)
	}
	return nil
}

// Preserve extended paths on Windows versions without automatic long-path support.
func protectionWindowsPath(path string) (string, error) {
	if len(path) < 248 || strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) {
		return path, nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(absolute, `\\`) {
		return `\\?\UNC\` + absolute[2:], nil
	}
	return `\\?\` + absolute, nil
}
