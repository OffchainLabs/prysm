### Fixed

- Treat a block as already processed during initial sync only if fork choice has it, so a block kept in the DB across a restart is imported again instead of skipped.
