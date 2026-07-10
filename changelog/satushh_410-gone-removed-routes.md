### Added

- Respond with `410 Gone` and a message naming the replacement route when a Beacon API route that was removed from the spec (e.g. `GET /eth/v1/beacon/blocks/{block_id}`, `POST /eth/v1/beacon/pool/attestations`, `GET /eth/v2/validator/blocks/{slot}`) is requested, instead of a bare `404 Not Found`.
