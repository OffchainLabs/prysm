### Fixed

- Derive `SECONDS_PER_SLOT` from `SLOT_DURATION_MS` (and vice versa) when a chain config file sets only one of them, and reject a file that sets both to contradictory values.