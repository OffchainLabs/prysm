### Added

- Add `GET`/`POST`/`DELETE /eth/v1/validator/{pubkey}/builders` keymanager endpoints for managing a key's per-builder configuration (keymanager-APIs #88).

### Changed

- Resolve per-key builder settings with field-level inheritance from `default_config` on v2 proposer settings; registration uses the default fee recipient when a key has none, and `--enable-builder` only applies when `enabled` is unset.
- Merge file- or URL-loaded proposer settings per key: the file only overrides keys it names.
- Setting a fee recipient, gas limit, or graffiti for a new key no longer copies `default_config`'s builder settings onto that key; the key inherits them and tracks later changes.

### Fixed

- Decode unset builder settings fields identically across both validator DB backends.

### Removed

- Remove the unused `relays` field from the proposer settings builder config; a settings file that still contains it logs a warning and the field is ignored.
