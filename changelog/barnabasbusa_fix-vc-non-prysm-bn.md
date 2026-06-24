### Fixed

- Fixed nil pointer dereference in validator client when connecting to non-Prysm beacon nodes (REST-only, no gRPC).
- Made initial proposer settings push non-fatal so the validator runner can start even if the beacon node is not yet ready at genesis.
