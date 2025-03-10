### Added 
- Added immediate broadcasting of proposer slashings when equivocating blocks are detected during block processing.
- Added signature verification for proposer slashing detection.
- Added a new `HeadStateErr` for testing equivocations

### Changed
- Improved equivocation detection by comparing blocks with the same slot and proposer against the head block.
- Removed redundant slashing verification checks in favor of using existing VerifyProposerSlashing function.