### Added

- Add `GET`/`POST`/`DELETE /eth/v1/validator/{pubkey}/builders` keymanager endpoints for managing a key's builder configuration (keymanager-APIs #88). The fee recipient, gas limit and graffiti endpoints are unchanged.
- Add per-key `min_bid` and `builder_boost_factor` to Gloas bid selection, and per-key `builder_boost_factor` to pre-Gloas block requests.
- Accept `POST /eth/v4/validator/blocks/{slot}` with inline builder preferences (beacon-APIs #625).

### Changed

- Resolve per-key builder config with field-level inheritance from `default_config`; `enabled` and `max_execution_payment` are presence-tracked so unset values inherit. Applies to v2 settings only — v1 settings keep the existing registration semantics unchanged.
- For v2 settings, per-key builder settings no longer require a per-key fee recipient to register; the default fee recipient is used.
- Send builder preferences inline with the block request instead of caching them on the beacon node; request auths sign opaque `auth_data` per builder-specs #165, with the builder URL carried separately.
- Merge file- or URL-loaded proposer settings per key: the file only overrides keys it names, and the settings schema version never regresses.
- Reject builder entries that duplicate a full `(url, auth_data, builder_pubkey)` tuple or repeat a `builder_pubkey`; a builder entry needs at least one of `url` or `builder_pubkey`.
- A per-key builders list replaces the validator client's builders; an empty list uses none (p2p bids only) while an omitted list inherits.
- For v2 settings, `--enable-builder` sets the default builder toggle only when it is unset; an explicit `enabled` and per-key entries are left to field-level inheritance.

- Enforce the per-builder `pubkey` from the validator config during bid selection: when set, only bids from that builder key are accepted for the entry's url.

### Fixed

- Normalize legacy proposer settings on load so unset builder `enabled` stays disabled and both validator DB backends decode `max_execution_payment` identically.
- Decode empty builder `auth_data`/`pubkey` identically to unset across both validator DB backends.
- Mask credentials in builder-client error messages and trace spans so a builder URL with userinfo or a query token cannot leak.
- Send the inline builder-preferences produce-block body as SSZ; the beacon node accepts SSZ or JSON.
- Validate the builder url on the ahead-of-time preferences endpoint before dialing.
- Preserve the winning builder url when unwrapping a stateless self-build produce response.

### Removed

- Remove the beacon-node builder-preference caches; `BlockRequest.builder_request_auths` is replaced by `builder_preferences`.
- Remove the unused `relays` field from the proposer settings builder config; a settings file that still contains it logs a warning and the field is ignored.
