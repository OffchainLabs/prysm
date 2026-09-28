### Added

- Implement the `POST /eth/v1/beacon/states/{state_id}/builders` beacon API endpoint (`getStateBuilders`), returning the Gloas builder registry filterable by builder IDs (pubkeys or indices) and statuses (`pending`, `active`, `exited`).

### Fixed

- Serialize the builder `version` field as a decimal string in beacon API responses, matching the spec's `Uint8` type, instead of a hex string.
- Accept beacon API `POST` requests with an empty body and no `Content-Type` header instead of rejecting them with `415 Unsupported Media Type`, so endpoints with an optional request body work with e.g. `curl -X POST`.
