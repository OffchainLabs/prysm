### Fixed

- Cold state migration no longer builds a state-diff boundary state from a block that was reorged out. The db retains blocks that lost fork choice, so when a boundary slot's nearest populated slot held only an orphan, that orphan was replayed into the persisted state.
