### Added

- Validator client now fetches duties mid-epoch for keys that become eligible after a keymanager reload or a doppelganger clearance, requesting only the newly eligible keys instead of waiting for the next epoch boundary.
