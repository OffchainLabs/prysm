### Fixed

- Prune Gloas execution payload envelopes (and their block-hash secondary index) during `DeleteHistoricalDataBeforeSlot`, so envelope storage no longer grows unbounded across finalization.
