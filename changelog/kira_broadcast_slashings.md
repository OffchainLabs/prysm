### Added 
- Added immediate broadcasting of proposer slashings when equivocating blocks are detected during block processing.
- Added dedicated block header verification for proposer slashing detection.

### Changed
- Improved equivocation detection by validating blocks against head block instead of cache.
- Removed reliance on seen block cache for slashing detection.

### Fixed
- Fixed potential false positives in equivocation detection by adding header validation.