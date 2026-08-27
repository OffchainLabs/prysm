### Fixed

- Accept `skip_randao_verification` with an empty value (`?skip_randao_verification`) in `/eth/v3/validator/blocks/{slot}` and `/eth/v4/validator/blocks/{slot}`, as specified by the Beacon API; `=true` is still accepted.
