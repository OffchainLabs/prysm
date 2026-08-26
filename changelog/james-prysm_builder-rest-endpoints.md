### Added

- Support POST on `/eth/v4/validator/blocks/{slot}` with a JSON- or SSZ-encoded `BuilderConfig` request body, and set the `Eth-Builder-Url` response header when an external builder bid wins, per beacon-APIs #630.
- Add `POST /eth/v1/validator/builder_preferences` to forward per-builder preference entries (JSON or SSZ) to their builders.
- Read the `Eth-Builder-Url` request header in the block publishing endpoints so the beacon node submits the signed block to the winning builder.

### Changed

- Builder API entry URLs are carried as bytes on the wire, enabling SSZ encoding of `BuilderConfig` and builder preference entries.
- `SubmitBuilderPreferences` now reports how many preference submissions failed instead of silently logging them.
