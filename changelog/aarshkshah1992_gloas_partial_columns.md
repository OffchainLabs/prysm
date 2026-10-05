### Added

- Added support for Gloas Partial Columns.

### Fixed

- Join every data column topic before broadcasting a proposal's columns, so partial columns on topics the proposer is not subscribed to are actually sent instead of silently reaching no peers.
