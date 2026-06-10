### Added

- Use the `engine_hasBlobs` Engine API method (when the EL advertises it via `engine_exchangeCapabilities`) to learn which blobs the EL is missing and eagerly publish header-only partial data columns whose parts metadata requests exactly those blobs, before fetching cells via `engine_getBlobsV3`. When `engine_hasBlobs` is unsupported, behavior is unchanged (GetBlobsV3 only).

### Changed

- Partial data column parts metadata now requests only the cells missing from the column instead of requesting all cells. This reduces redundant cell pushes from peers for all partial-column-enabled nodes, independent of `engine_hasBlobs` support.
