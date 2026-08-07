### Added

- Add `GET`/`POST`/`DELETE /eth/v1/validator/{pubkey}/builders` keymanager endpoints for per-key builder configuration (keymanager-APIs #88).

### Changed

- Add v2 proposer settings (`"version": 2`): builder fields a key does not set inherit from `default_config`. `--enable-builder` still forces the default builder on; a key with explicit `enabled: false` stays opted out.
- Setting a fee recipient, gas limit, or graffiti no longer snapshots `default_config`'s builder settings onto that key; the key keeps following the default as it changes.

### Fixed

- Fix the minimal slashing protection database decoding unset builder settings fields as explicit zero values; both validator DB backends now read the same stored settings identically.

### Removed

- Remove the unused `relays` field from the builder config; a settings file that still contains it logs a warning and is otherwise unaffected.
