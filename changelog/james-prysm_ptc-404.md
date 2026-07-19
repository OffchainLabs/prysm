### Changed

- GET /eth/v1/validator/payload_attestation_data/{slot} now returns 204 instead of 503 when no block has been seen for the requested slot, per ethereum/beacon-APIs#612
