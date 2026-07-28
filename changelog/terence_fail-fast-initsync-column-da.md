### Fixed

- Fail the block data column availability check during initial sync instead of waiting indefinitely, so checkpoint sync no longer stalls when a block's custody columns are missing.
