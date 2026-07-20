### Added

- Add `GET`/`POST /eth/v1/validator/config` keymanager endpoints (keymanager-APIs #87), managing fee recipient, target gas limit, graffiti, and per-key builder preferences (builder list, `min_bid`, `max_execution_payment`, `builder_boost_factor`, proxy) as one atomic per-key document. POST replaces a key's fragment, an empty document clears it, and each key is applied independently with a `set`/`not_found`/`error` status.
- Wire per-key `builder_boost_factor` and `min_bid` into Gloas builder-API bid selection, and `builder_boost_factor` into the pre-Gloas `builder_boost_factor` block-production query.
- Accept `POST /eth/v4/validator/blocks/{slot}` carrying inline per-builder preferences (beacon-APIs #625 JIT model).

### Changed

- Proposer builder config `enabled` and `max_execution_payment` are now presence-tracked: unset values inherit instead of defaulting (explicit `max_execution_payment: 0` still means trustless-only).
- Per-key builder config now resolves with field-level inheritance from `default_config`: unset fields inherit the default's values, and a present `builders`/`relays` list replaces the default's. Per-key builder settings no longer require a per-key fee recipient for validator registration (the default fee recipient is inherited).
- Builder preferences now travel inline with the block request (JIT) instead of being pushed ahead to a beacon-node cache; request auths sign the opaque per-builder `auth_data` (builder-specs #165) with the builder URL carried explicitly.

### Fixed

- Normalize legacy (pre-v2) proposer settings on load and at the v2 upgrade: wire-absent builder `enabled` becomes an explicit `false` (a previously disabled per-key builder section can never silently activate through inheritance), and the filesystem backend's spurious explicit-0 `max_execution_payment` is cleared so both database backends decode legacy data identically.

### Removed

- Remove the `SubmitBuilderPreferences` internal gRPC RPC and the beacon-node builder-preference caches; `BlockRequest` field `builder_request_auths` is replaced by `builder_preferences`.
