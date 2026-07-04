### Added

- Add `max_execution_payment` to the proposer builder config, the maximum execution-layer payment a proposer will accept from a Gloas builder.

### Changed

- Retain `BuilderConfig` (relays, enabled, max execution payment) when upgrading proposer settings from v1 to v2 instead of dropping it.

### Removed

- Remove the unused top-level `max_execution_payment` proposer-option field in favor of the per-builder `BuilderConfig.max_execution_payment`.
