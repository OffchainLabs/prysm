### Fixed

- Replay historical states from an earlier available state when required hierarchical diffs or snapshots are missing, without treating corrupt records as missing or skipping required ancestors.

### Changed

- Check required hierarchical diff records before loading full snapshots to avoid repeated large decodes while searching for available historical states.
