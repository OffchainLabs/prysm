### Added

- Support for checkpoint syncing from an unfinalized checkpoint.

### Fixed

- Checkpoint sync no longer makes justified == finalized.
- `SaveOrigin` now verifies that the origin block matches the origin state's `latest_block_header` rather than trusting the pair.
