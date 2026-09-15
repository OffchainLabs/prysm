### Fixed

- Advance the cold-state migration cursor even when the epoch boundary cache has no state for the finalized root, so migration keeps progressing on freshly started nodes and during long non-finalization.
