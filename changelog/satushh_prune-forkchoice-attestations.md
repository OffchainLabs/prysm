### Fixed

- Prune expired fork choice attestations from the attestation pool, so attestations for blocks the node never imports no longer stay in memory until restart.
