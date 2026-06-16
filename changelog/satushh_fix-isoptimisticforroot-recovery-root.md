### Fixed

- `IsOptimisticForRoot` now recovers the validated checkpoint's state summary using the checkpoint root instead of the queried block's root, so the optimism slot comparison is correct.
