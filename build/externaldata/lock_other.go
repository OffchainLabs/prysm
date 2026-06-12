//go:build !unix

package externaldata

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
)

// fileLock is a best-effort lock on non-unix platforms: there is no cross-process
// flock, but the in-process sync.Once still serializes fetches within a process.
type fileLock struct{ f *os.File }

func acquireLock(path string) (*fileLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}

	return &fileLock{f: f}, nil
}

func (l *fileLock) release() {
	if err := l.f.Close(); err != nil {
		logrus.WithError(err).WithField("path", l.f.Name()).Warn("Failed to close lock file")
	}
}
