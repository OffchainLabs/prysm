### Changed

- GET /eth/v1/validator/payload_attestation_data/{slot} now returns 404 instead of 503 when no canonical block has been seen for the requested slot
