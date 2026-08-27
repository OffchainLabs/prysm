### Added

- Per-validator `validator_failed_envelope_submissions` metric counting failed self-build envelope submissions (omitted under `--disable-account-metrics`).

### Fixed

- Execution payload envelope publishing now selects the beacon node's precomputed data column sidecars by envelope root, and reuses them when the locally built envelope is published with blob data, skipping redundant verification and recomputation.
- Validator client now records the proposal log and metrics for an accepted Gloas block even when the follow-up envelope publish fails.
