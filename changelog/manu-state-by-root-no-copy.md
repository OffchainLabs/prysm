### Changed

- `StateByRootIfCachedNoCopy` now also checks the epoch boundary state cache.
- Use `state.ReadOnlyBeaconState` instead of state.BeaconState when possible.
