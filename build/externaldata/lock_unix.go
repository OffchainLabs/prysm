//go:build unix

package externaldata

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// fileLock is an advisory cross-process lock backed by flock(2).
type fileLock struct{ f *os.File }

func acquireLock(path string) (*fileLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}

	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		if cerr := f.Close(); cerr != nil {
			logrus.WithError(cerr).WithField("path", path).Warn("Failed to close lock file after flock error")
		}

		return nil, fmt.Errorf("acquiring lock: %w", err)
	}

	return &fileLock{f: f}, nil
}

func (l *fileLock) release() {
	if err := unix.Flock(int(l.f.Fd()), unix.LOCK_UN); err != nil {
		logrus.WithError(err).WithField("path", l.f.Name()).Warn("Failed to release file lock")
	}

	if err := l.f.Close(); err != nil {
		logrus.WithError(err).WithField("path", l.f.Name()).Warn("Failed to close lock file")
	}
}
