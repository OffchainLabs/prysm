### Fixed

- Return an error for an invalid `--p2p-local-ip` instead of silently ignoring it (the previous code wrapped a nil error and returned no options change).
