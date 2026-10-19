### Fixed

- Reject graffiti longer than 32 bytes in the keymanager API `POST /eth/v1/validator/{pubkey}/graffiti` with status 400. Before, the value was stored and then truncated or rejected at proposal time.
