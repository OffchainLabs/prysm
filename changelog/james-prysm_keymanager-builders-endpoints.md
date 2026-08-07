### Added

- Add `GET`/`POST`/`DELETE /eth/v1/validator/{pubkey}/builders` keymanager endpoints (keymanager-APIs #88).

### Changed

- Add v2 proposer settings: per-key builder fields inherit from `default_config`; `--enable-builder` forces the default builder on, and per-key `enabled: false` still opts a key out.
- Merge file- or URL-loaded proposer settings per key; the file only overrides keys it names.
- Setting a fee recipient, gas limit, or graffiti no longer copies `default_config`'s builder settings onto the key.

### Fixed

- Decode unset builder settings fields identically across both validator DB backends.

### Removed

- Remove the unused `relays` field from the builder config; a file that still contains it logs a warning and the field is ignored.
