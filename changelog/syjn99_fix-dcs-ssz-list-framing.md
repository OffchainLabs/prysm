### Fixed

- Encode the SSZ response of `GET /eth/v1/debug/beacon/data_column_sidecars/{block_id}` as an SSZ list of variable-size elements (4-byte offsets followed by the elements) instead of plain concatenation, per [beacon-APIs#633](https://github.com/ethereum/beacon-APIs/pull/633).
