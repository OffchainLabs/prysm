### Added

- `--submit-blacklisted-builder-bids` hidden feature flag to skip the builder circuit breaker check in `SubmitSignedExecutionPayloadBid`, so a blacklisted builder can still broadcast its bid for testing that peers do not propagate it.
