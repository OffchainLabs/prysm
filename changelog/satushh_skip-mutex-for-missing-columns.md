### Fixed

- Return early from data column `Get` when nothing is stored for the root, instead of first creating a per-root file mutex that stays until a later prune clears them all.
