### Fixed

- Keymanager API `GET /eth/v1/validator/{pubkey}/gas_limit` now returns the scheduled network gas limit from Gloas on when the validator has no gas limit of its own, matching what the validator client signs.
