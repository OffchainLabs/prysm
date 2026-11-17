### Added

- State-diff metrics for writes, reads, anchor cache, startup, pruning, hot snapshots and errors.

### Changed

- The two `db_beacon_state_*` summaries are now histograms.
- The `state_diff_get_anchor_state_*` histograms use millisecond buckets.
- `StateBySlotFromDiffTree` reports a missing level-0 snapshot as state not found.
