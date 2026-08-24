### Changed

- Validator client now forces stateless block production (Gloas and later) when it is configured with several beacon nodes, since only the node that built a block can reveal its execution payload. An explicit `--stateless=false` is ignored in this setup with a warning.
