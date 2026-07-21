### Added

- Add `GET`/`POST /eth/v1/validator/config` keymanager endpoints for managing per-key proposer and builder preferences as a single document (keymanager-APIs #87).
- Add per-key `min_bid` and `builder_boost_factor` to Gloas bid selection, and per-key `builder_boost_factor` to pre-Gloas block requests.
- Accept `POST /eth/v4/validator/blocks/{slot}` with inline builder preferences (beacon-APIs #625).

### Changed

- Resolve per-key builder config with field-level inheritance from `default_config`; `enabled` and `max_execution_payment` are presence-tracked so unset values inherit.
- Per-key builder settings no longer require a per-key fee recipient to register; the default fee recipient is used.
- Send builder preferences inline with the block request instead of caching them on the beacon node; request auths sign opaque `auth_data` per builder-specs #165, with the builder URL carried separately.
- Merge file- or URL-loaded proposer settings per key: the file only overrides keys it names, and the settings schema version never regresses.

### Fixed

- Normalize legacy proposer settings on load so unset builder `enabled` stays disabled and both validator DB backends decode `max_execution_payment` identically.

### Removed

- Remove the beacon-node builder-preference caches; `BlockRequest.builder_request_auths` is replaced by `builder_preferences`.
