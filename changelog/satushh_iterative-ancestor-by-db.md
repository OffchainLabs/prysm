### Fixed

- Walk ancestors with a loop instead of recursion when `Ancestor` falls back to the DB, so stack use no longer grows with the number of blocks walked.
