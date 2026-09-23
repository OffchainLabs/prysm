### Fixed

- Validator clients keep requesting payload attestation data for 500ms past the PTC due time instead of dropping the vote after one failed request, such as when the beacon node's clock lags or a REST beacon node stalls.
