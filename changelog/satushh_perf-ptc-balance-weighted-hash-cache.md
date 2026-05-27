### Changed

- Cache random_bytes across 16 rounds of `compute_balance_weighted_selection`, mirroring consensus-specs PR#5079. Cuts `ProcessPTCWindow` wall time.
