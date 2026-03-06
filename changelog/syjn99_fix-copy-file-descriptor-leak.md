### Fixed

- Fix file descriptor leaks in `CopyFile`: close both source and destination files with deferred close.
