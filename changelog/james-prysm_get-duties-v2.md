### Added

- GetDutiesV2 gRPC function, removes committee list from duties, replaced with committee length, validator committee index.

### Changed

- Updated validator to use GetDutiesV2. This should improve the deserialization of the duties response as we don't actually use the full committee list on the validator client side.
- GetDuties returns validator status unknown if the validator is not found in the state instead of trying to recalculate the status.