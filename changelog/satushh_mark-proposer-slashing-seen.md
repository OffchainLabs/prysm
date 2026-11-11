### Fixed

- Mark a gossiped proposer slashing's proposer as seen before inserting it into the pool, so later slashings for that proposer are ignored even when the pool already holds one.
