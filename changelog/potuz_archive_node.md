### Added

- `--enable-archive` turns the node into an archive node: it backfills blocks down to `--archive-origin-state` (genesis by default) and regenerates every historical state into the state-diff tree. Implies `--enable-state-diff` and `--enable-backfill`.
