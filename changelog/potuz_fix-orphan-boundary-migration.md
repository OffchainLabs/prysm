### Fixed

- Cold state migration no longer builds a state-diff boundary state from a block that was reorged out. Neither the db nor the epoch boundary state cache drops a block that merely lost fork choice, so a boundary slot whose nearest populated slot held only an orphan, or whose cache entry named a reorged sibling, replayed that orphan into the persisted state.
