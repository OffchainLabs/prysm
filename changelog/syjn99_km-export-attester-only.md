### Fixed

- Slashing protection export no longer returns empty data when a key has attestations but no proposer history, so keymanager DELETE `/eth/v1/keystores` no longer returns `"null"` slashing protection.
