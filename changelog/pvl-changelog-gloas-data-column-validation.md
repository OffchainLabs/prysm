### Changed

- Refactor Gloas `data_column_sidecar` gossip validation into dedicated sync and verification paths, verify against `bid.blob_kzg_commitments`, and dedupe by `(beacon_block_root, index)`.
