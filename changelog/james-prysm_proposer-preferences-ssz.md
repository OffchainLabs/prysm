### Added

- Accept SSZ (`application/octet-stream`) request bodies on `POST /eth/v1/validator/proposer_preferences`, matching beacon-APIs #608.

### Changed

- Validator client submits proposer preferences as SSZ by default, falling back to JSON if the beacon node returns 415.
