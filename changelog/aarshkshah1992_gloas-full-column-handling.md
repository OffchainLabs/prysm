### Fixed

- Gloas data column sidecars received over gossip are now imported by the sync subscriber instead of erroring on a Fulu-only `ProposerIndex()` accessor, which previously dropped every Gloas full column before it reached the chain. The Fulu republish-on-partial-extension path was also restored to run before the column is marked seen.
- Backfill no longer fails validation of Gloas-era data columns: the Fulu-only KZG-commitment and signed-block-header cross-checks are skipped for Gloas sidecars (which carry neither on the wire; commitments come from the block's bid).
