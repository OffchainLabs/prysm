### Fixed

- Mark a gossiped attester slashing's validators as seen before inserting it into the pool, so later slashings for those validators are ignored even when the pool already holds one.
