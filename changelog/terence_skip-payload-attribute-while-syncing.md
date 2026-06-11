### Fixed

- Skip payload attribute computation in `saveHeadIfNeeded` while syncing. Computing attributes with a head state far behind the wall clock processed thousands of slots per synced block, slowing initial sync to ~0.1 blocks/s.
