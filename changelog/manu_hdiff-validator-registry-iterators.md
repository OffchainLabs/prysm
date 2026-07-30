### Changed

- State diff: walk validator registries with iterators when computing a diff. `diffToVals` no longer
  calls `ValidatorsReadOnly` on both states, which materialized the whole registry as a slice twice
  (~1.2GB per state-diff save on mainnet).
