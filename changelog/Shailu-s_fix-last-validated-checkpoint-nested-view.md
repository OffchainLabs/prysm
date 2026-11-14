### Fixed

- Fix a possible database deadlock at startup when `LastValidatedCheckpoint` falls back to the finalized checkpoint while a concurrent write remaps the DB.
