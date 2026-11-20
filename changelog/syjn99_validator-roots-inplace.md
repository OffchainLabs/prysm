### Changed

- Compute validator roots in place in `OptimizedValidatorRoots`, and skip the validator copy in the full state hash tree root when no state has its own validator values. This removes about 5 heap allocations per validator and the per-level output slices from the validator registry hash tree root.
