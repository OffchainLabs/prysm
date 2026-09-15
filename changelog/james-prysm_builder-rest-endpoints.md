### Added

- Support POST on `/eth/v4/validator/blocks/{slot}` with a `BuilderConfig` body and return the winning builder in the `Eth-Builder-Url` response header, per beacon-APIs #630.
- Add `POST /eth/v1/validator/builder_preferences` to forward per-builder preference entries to their builders, reporting per-entry failures in the 400 response per the spec's `IndexedErrorMessage`.
- Read the `Eth-Builder-Url` request header in the block publishing endpoints to submit the signed block to the winning builder.

### Changed

- Per-URL builder clients no longer follow HTTP redirects, per the beacon-APIs builder URL requirements.
