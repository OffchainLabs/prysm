### Added
- prometheus histogram `cells_and_proofs_from_structured_computation_milliseconds` to track computation time for cells and proofs from structured blobs.
- prometheus histogram `get_blobs_v2_latency_milliseconds` to track RPC latency for `getBlobsV2` calls to the execution layer.

### Changed
- Run `ComputeCellsAndProofsFromFlat` in parallel to improve performance when computing cells and proofs.
- Run `ComputeCellsAndProofsFromStructured` in parallel to improve performance when computing cells and proofs.
