### Fixed

- Accept SSZ (`application/octet-stream`) request bodies on `POST /eth/v1/beacon/pool/payload_attestations`. The handler already decoded SSZ, but the route only registered JSON so SSZ submissions were rejected with 415.
- Allow SSZ responses on `GET /eth/v1/beacon/pool/payload_attestations`. The handler already supported SSZ, but the route's Accept negotiation only allowed JSON, making the SSZ response unreachable.

### Changed

- Validator client submits payload attestations as SSZ by default, falling back to JSON if the beacon node returns 415.
- Validator client requests `payload_attestation_data` as SSZ, decoding either an SSZ or JSON response.
