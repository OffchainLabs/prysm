### Fixed

- Skip the missing block index cleanup when a slot range read finds no missing blocks, so `Blocks` no longer commits an empty BoltDB write transaction on every by-range query.
