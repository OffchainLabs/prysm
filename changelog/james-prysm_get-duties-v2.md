### Added

- GetDutiesV2 gRPC function, removes committee list from duties, replaced with committee length, validator committee index.

### Changed

- GetDuties returns validator status unknown if the validator is not found in the state instead of trying to recalculate the status.