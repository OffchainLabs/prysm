### Added

- Added support for otel tracing transport in HTTP clients in Prysm. This allows for tracing headers to be sent with http requests such that spans between the validator and beacon chain can be connected in the tracing graph. This change does nothing without `--enable-tracing`.
