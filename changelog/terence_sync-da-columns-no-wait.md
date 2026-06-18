### Fixed

- Don't block on the gossip data column availability wait for past-slot blocks/envelopes during sync; fail fast so the caller refetches.
