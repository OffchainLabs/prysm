### Fixed

- `/prysm/v1/validators/head/active_set_changes`: `ActivatedValidatorIndices` now returns only validators activated in the requested epoch (previously it returned all active validators).

### Changed

- `/eth/v1/beacon/states/{state_id}/validators`: Avoid full validators list (~2.3M on mainnet) materialization by iterating over validators via `ValidatorsReadOnlySeq`.
- `/eth/v1/beacon/states/{state_id}/validator_identities` (JSON response): Avoid full validators list (~2.3M on mainnet) materialization by iterating over validators via `ValidatorsReadOnlySeq`.
- `/eth/v1/beacon/states/{state_id}/validator_identities` (SSZ response): Avoid full validators list (~2.3M on mainnet) materialization by iterating over validators via `ValidatorsReadOnlySeq`.
- `/eth/v1/beacon/states/{state_id}/validator_count`: Avoid full validators list (~2.3M on mainnet) materialization by iterating over validators via `ValidatorsReadOnlySeq`.
- `/prysm/v1/validators/head/active_set_changes`: Avoid full validators list (~2.3M on mainnet) materialization by iterating over validators via `ValidatorsReadOnlySeq`.
