### Added

- Add `--postpone-shutdown-for-proposals` flag. When set, a graceful shutdown signal (SIGINT/SIGTERM, e.g. Ctrl-C on Linux) is postponed while a validator controlled by a connected validator client still has a block proposal duty in the current or next epoch.