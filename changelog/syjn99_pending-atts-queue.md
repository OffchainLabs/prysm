### Changed

- Replaced the pending attestations queue mutex with a channel-fed queue owned by a single goroutine.

### Fixed

- Fixed a race where attestations for a block imported during gossip validation could strand in the pending queue until pruned.
