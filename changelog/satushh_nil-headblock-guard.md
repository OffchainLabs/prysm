### Fixed
- Guard the startup head block with `blocks.BeaconBlockIsNil` before use: `BeaconDB.Block` returns a nil block with no error when the root is not found, so the previous inner-block nil check still segfaulted the node at startup.
