package filesystem

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/io/file"
)

type protectionFile interface {
	io.Writer
	Chmod(os.FileMode) error
	Sync() error
	Close() error
	Name() string
}

// protectionPersistence keeps fault injection local to one store or write.
type protectionPersistence struct {
	createTemp func(string, string) (protectionFile, error)
	openDir    func(string) (protectionFile, error)
	rename     func(string, string) error
	remove     func(string) error
}

func defaultProtectionPersistence() protectionPersistence {
	return protectionPersistence{
		createTemp: func(dir, pattern string) (protectionFile, error) { return os.CreateTemp(dir, pattern) },
		openDir: func(path string) (protectionFile, error) {
			return os.Open(path) // #nosec G304 -- path is derived from the configured database directory and opened only for directory synchronization.
		},
		rename: os.Rename,
		remove: os.Remove,
	}
}

func (s *Store) protectionWriter() protectionPersistence {
	if s.protectionPersistence != nil {
		return *s.protectionPersistence
	}
	return defaultProtectionPersistence()
}

func protectionDurabilitySupported() error {
	switch runtime.GOOS {
	case "aix", "darwin", "dragonfly", "freebsd", "illumos", "linux", "netbsd", "openbsd", "solaris":
		return nil
	default:
		return fmt.Errorf("durable slashing protection writes are unsupported on %s", runtime.GOOS)
	}
}

func (p protectionPersistence) write(path string, data []byte) (err error) {
	if err = protectionDurabilitySupported(); err != nil {
		return err
	}
	path, err = file.ExpandPath(path)
	if err != nil {
		return err
	}
	exists, err := file.Exists(path, file.Regular)
	if err != nil {
		return err
	}
	if exists {
		allowed, err := file.HasReadWritePermissions(path)
		if err != nil {
			return err
		}
		if !allowed {
			return fmt.Errorf("file already exists without proper 0600 permissions: %s", path)
		}
	}
	temp, err := p.createTemp(filepath.Dir(path), ".slashing-protection-*")
	if err != nil {
		return fmt.Errorf("create temporary protection file: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			if closeErr := temp.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close temporary protection file: %w", closeErr))
			}
		}
		if removeErr := p.remove(temp.Name()); removeErr != nil && !os.IsNotExist(removeErr) {
			err = errors.Join(err, fmt.Errorf("remove temporary protection file: %w", removeErr))
		}
	}()
	if err = temp.Chmod(params.BeaconIoConfig().ReadWritePermissions); err != nil {
		return fmt.Errorf("chmod temporary protection file: %w", err)
	}
	n, err := temp.Write(data)
	if err != nil {
		return fmt.Errorf("write temporary protection file: %w", err)
	}
	if n != len(data) {
		return fmt.Errorf("write temporary protection file: %w", io.ErrShortWrite)
	}
	if err = temp.Sync(); err != nil {
		return fmt.Errorf("sync temporary protection file: %w", err)
	}
	closed = true
	if err = temp.Close(); err != nil {
		return fmt.Errorf("close temporary protection file: %w", err)
	}
	if err = p.rename(temp.Name(), path); err != nil {
		return fmt.Errorf("rename protection file: %w", err)
	}
	return p.syncDirectories(filepath.Dir(path))
}

// Sync ancestors on every retry, even when an earlier attempt created the directories.
func (p protectionPersistence) syncDirectories(path string) error {
	if err := protectionDurabilitySupported(); err != nil {
		return err
	}
	path, err := file.ExpandPath(path)
	if err != nil {
		return err
	}
	for {
		dir, err := p.openDir(path)
		if err != nil {
			return fmt.Errorf("open protection directory %s: %w", path, err)
		}
		syncErr := dir.Sync()
		closeErr := dir.Close()
		if syncErr != nil || closeErr != nil {
			return fmt.Errorf("sync/close protection directory %s: %w", path, errors.Join(syncErr, closeErr))
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}
