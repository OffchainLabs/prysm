### Changed

- Validator client now enables Gloas stateless block production automatically when it is configured with several beacon nodes, since only the node that built a block can reveal its execution payload. Pass `--stateless=false` to keep it off.
