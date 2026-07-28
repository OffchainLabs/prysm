### Fixed

- Fix a panic in `GET /eth/v1/beacon/blobs/{block_id}` when a `versioned_hashes` filter matches a commitment that appears more than once in the block.
