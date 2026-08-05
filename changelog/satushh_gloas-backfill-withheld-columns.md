### Fixed

- Fix backfill handling of gloas withheld-payload data columns: classify payload fullness as revealed, withheld, or unknown instead of assuming the batch-tail block is revealed, resolve an unknown tail against the already-imported child at `BackfillStatus.LowRoot` before import, and exclude withheld blocks from the data column availability check so batches no longer wedge in `batchSyncColumns` or fail import forever.
