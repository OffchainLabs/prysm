### Fixed

- Add the fork-choice read lock when reading optimistic status, avoiding a concurrent map read/write crash.
