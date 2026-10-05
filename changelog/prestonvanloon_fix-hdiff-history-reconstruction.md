### Fixed

- Replay historical states from an earlier available state when required hierarchical diffs or snapshots are missing, without treating corrupt records as missing or skipping required ancestors.
- Ignore cached anchors for empty hierarchical diff levels without skipping required ancestor diffs.

### Changed

- Check required hierarchical diff records before loading full snapshots to avoid repeated large decodes while searching for available historical states.
