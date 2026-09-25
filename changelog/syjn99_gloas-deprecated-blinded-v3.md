### Changed

- Return 400 instead of 500 from `GET /eth/v1/beacon/blinded_blocks/{block_id}` for Gloas blocks, and from `GET /eth/v3/validator/blocks/{slot}` for Gloas slots, per beacon-APIs#651.
