### Changed

- Compute validator roots in place in `OptimizedValidatorRoots`. This removes about 5 heap allocations per validator and the per-level output slices from the validator registry hash tree root.
