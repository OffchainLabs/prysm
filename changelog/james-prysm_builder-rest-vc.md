### Added

- Connect the REST validator client to the Gloas builder endpoints: block requests carry the `BuilderConfig` via POST, the `Eth-Builder-Url` header is read from block production and echoed on publish, and builder preferences are submitted to `POST /eth/v1/validator/builder_preferences`.

### Fixed

- Builder entries loaded from proposer settings files, URLs, or the validator database are now checked against the spec size and format limits at startup; invalid entries are dropped with a warning instead of failing block production requests.

### Removed

- The GET variant of `/eth/v4/validator/blocks/{slot}`: per the beacon-APIs spec the produce request is POST-only and always carries a `BuilderConfig` body.
