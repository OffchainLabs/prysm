### Added

- Connect the REST validator client to the Gloas builder endpoints: block requests carry the `BuilderConfig` via POST, the `Eth-Builder-Url` header is read from block production and echoed on publish, and builder preferences are submitted to `POST /eth/v1/validator/builder_preferences`.
