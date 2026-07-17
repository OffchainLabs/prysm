### Changed

- Implement the active-active validator client: Using the `--enable-beacon-rest-api` flag,
  if multiple beacon nodes are provided in the `--beacon-rest-api-provider` flag, then the
  validator client will use the best suited connected beacon node to attest,
  participate in sync committees and propose blocks.
