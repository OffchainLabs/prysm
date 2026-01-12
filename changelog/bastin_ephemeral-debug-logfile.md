### Added

- Added an ephemeral debug logfile that for beacon and validator nodes that captures debug-level logs for 24 hours. It
  also keeps 1 backup of in case of size-based rotation. The logfiles are stored in `datadir/logs/`. This feature is
  enabled by default and can be disabled by setting the `--disable-ephemeral-log-file` flag.