### Added

- Add `GET`/`POST`/`DELETE /eth/v1/validator/{pubkey}/builders` keymanager endpoints for per-key builder configuration (keymanager-APIs #88).

### Changed

- Add v2 proposer settings (`"version": 2`): builder fields a key does not set inherit from `default_config`. v2 has no `enabled` field; a key participates in builder registration when its resolved `builders` list names at least one builder, and an explicit empty `builders` list opts the key out.
- v1 builder settings are not migrated to v2: at the gloas fork fee recipients, gas limits, and graffiti carry over and v1 builder content is replaced with defaults, with a warning. `--enable-builder` has no effect with v2 settings.
- Setting a fee recipient, gas limit, or graffiti no longer snapshots `default_config`'s builder settings onto that key; the key keeps following the default as it changes.

### Fixed

- Fix the minimal slashing protection database decoding unset builder settings fields as explicit zero values; both validator DB backends now read the same stored settings identically.

### Removed

- Remove the unused `relays` field from the builder config; a settings file that still contains it is unaffected.
