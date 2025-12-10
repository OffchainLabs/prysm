### Added
- Data column backfill.
- Backfill metrics for columns: backfill_data_column_sidecar_downloaded, backfill_data_column_sidecar_downloaded_bytes, backfill_batch_columns_download_ms, backfill_batch_columns_verify_ms.

### Changed
- backfill metrics that changed name and/or histogram buckets: backfill_batch_time_verify -> backfill_batch_verify_ms, backfill_batch_time_waiting -> backfill_batch_waiting_ms, backfill_batch_time_roundtrip -> backfill_batch_roundtrip_ms, backfill_blocks_bytes_downloaded -> backfill_blocks_downloaded_bytes, backfill_batch_time_verify -> backfill_batch_verify_ms, backfill_batch_blocks_time_download -> backfill_batch_blocks_download_ms, backfill_batch_blobs_time_download -> backfill_batch_blobs_download_ms, backfill_blobs_bytes_downloaded -> backfill_blocks_downloaded_bytes, 

