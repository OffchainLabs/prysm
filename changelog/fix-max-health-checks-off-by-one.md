### Fixed

- `--max-health-checks` now exits the validator on the configured number of consecutive failed health checks instead of one check later. Setting `--max-health-checks=1` exits on the first failed check.
