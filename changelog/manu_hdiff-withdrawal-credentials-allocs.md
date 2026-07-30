### Changed

- State diff: compare validator withdrawal credentials without allocating. `diffToVals` no longer
  allocates two 32-byte slices per validator, removing ~4.6M allocations (~150MB of garbage) from
  every state-diff save on mainnet.
