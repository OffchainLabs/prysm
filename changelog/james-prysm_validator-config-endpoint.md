### Added

- Add `GET`/`POST /eth/v1/validator/config` keymanager endpoints (keymanager-APIs #87), managing fee recipient, target gas limit, graffiti, and per-key builder preferences (builder list, `min_bid`, `max_execution_payment`, `builder_boost_factor`, proxy) as one atomic per-key document. POST replaces a key's fragment, an empty document clears it, and each key is applied independently with a `set`/`not_found`/`error` status.
- Wire per-key `builder_boost_factor` and `min_bid` into Gloas builder-API bid selection, and `builder_boost_factor` into the pre-Gloas `builder_boost_factor` block-production query.

### Changed

- Proposer builder config `enabled` is now presence-tracked so an unset value inherits from `default_config` instead of defaulting to disabled.
