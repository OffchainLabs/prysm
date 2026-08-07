### Fixed

- Validator monitor: use `continue` instead of `break` when skipping validators with no recorded performance, so a single validator without data no longer suppresses the "Aggregated performance since launch" log for all other monitored validators.
