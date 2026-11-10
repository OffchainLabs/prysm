### Fixed

- Return one blob per requested versioned hash from `GET /eth/v1/beacon/blobs/{block_id}` when a block repeats a commitment, using the first matching index, per beacon-APIs#676.
