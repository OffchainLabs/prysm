### Added

- Add `GET`/`POST`/`DELETE /eth/v1/validator/{pubkey}/builders` keymanager endpoints for per-key builder configuration (keymanager-APIs #88). The endpoints respond 501 on networks without a scheduled gloas fork, where builder configuration cannot take effect.

### Changed

- Add v2 proposer settings (`"version": 2`): builder fields a key does not set inherit from `default_config`; v2 has no `enabled` field. A settings source without a `version` that contains v2 builder fields (`builders`, `min_bid`, `builder_boost_factor`, `max_execution_payment`) is treated as version 2 automatically. Proposer-settings semantics are keyed on the gloas fork, not the schema version: before the fork the legacy mev-boost rules apply (with a resolved nonempty `builders` list also opting a key in, and an explicit empty list opting it out), and from the fork on only v2 content is read. Writing builders via the keymanager API never changes other keys' behavior or drops v1 content.
- v1 builder settings are not migrated to v2: at the gloas fork fee recipients and graffiti carry over, while v1 builder content — including its gas limits — is dropped and replaced with defaults, with a warning. Gas limits apply post-fork only when explicitly set at the option level, so validators follow future chain-default gas limit increases unless they opt out.
- The gas-limit keymanager API writes the option-level gas limit and no longer requires an enabled builder; deleting a gas limit unsets it (following the chain default) instead of pinning the current default value. Builder registration resolves fee recipients and participation independently, so a key with an enabled builder and only a default fee recipient now registers.
- Setting a fee recipient, gas limit, or graffiti no longer snapshots `default_config`'s builder settings onto that key; the key keeps following the default as it changes.

### Deprecated

- `--with-builder` generates legacy (pre-gloas) mev-boost builder settings, which are discontinued at the gloas fork; the command now warns when the flag is used.

### Fixed

- Fix the minimal slashing protection database decoding unset builder settings fields as explicit zero values; both validator DB backends now read the same stored settings identically.

### Removed

- Remove the unused `relays` field from the builder config; a settings file that still contains it is unaffected.
