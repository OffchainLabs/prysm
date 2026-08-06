### Changed

- Implement the active-active validator client: Using the `--enable-beacon-rest-api` flag,
  if multiple beacon nodes are provided in the `--beacon-rest-api-provider` flag, then the
  validator client will use the best suited connected beacon node to attest,
  participate in sync committees and propose blocks.
  This replaces the previous active-passive connection scheme, where only one beacon node
  was used at a time and the validator client failed over to another one only when the
  active node became unavailable.

### Removed

- Remove the active-passive connection scheme in the REST validator client, in favor of the
  new active-active connection scheme.
