### Fixed

- Clear the origin checkpoint block root pointer in `DeleteHistoricalDataBeforeSlot` when the origin block has been pruned, so `OriginCheckpointBlockRoot` no longer returns a dangling root.
