### Fixed

- Fixed a nil-pointer panic in the backfill worker pool when retrying a batch whose blob/column sync construction failed after block verification (e.g. a `CustodyGroupCount` error in `newColumnSync`).
