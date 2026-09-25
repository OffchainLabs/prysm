### Added

- Broadcast a pending block on the fork digest of its own slot instead of the current fork's, so a block resolved from the pending queue after a fork transition is not sent on a topic where peers cannot decode it.