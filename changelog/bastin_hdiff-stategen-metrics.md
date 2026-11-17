### Added

- `migrate_to_cold_*`, `stategen_archive_pending`, and `stategen_load_state_milliseconds` metrics.
- Migration latency is recorded for successful, non-no-op calls; failed calls increment `migrate_to_cold_failures_total`.

### Changed

- `replay_blocks_milliseconds`, `replay_to_slot_milliseconds`, `save_state_to_cold_milliseconds`, `on_block_processing_milliseconds`, and `chain_service_processing_milliseconds` are now histograms; `replay_blocks_count` covers all replay paths.
