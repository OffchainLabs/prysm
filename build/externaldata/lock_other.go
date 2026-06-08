//go:build !unix

package externaldata

import "os"

// fileLock is a best-effort lock on non-unix platforms: there is no cross-process
// flock, but the in-process sync.Once still serializes fetches within a process.
type fileLock struct{ f *os.File }

func acquireLock(path string) (*fileLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	return &fileLock{f: f}, nil
}

func (l *fileLock) release() { _ = l.f.Close() }
