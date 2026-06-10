### Fixed

- Run the `(slot, builder_index)` dedup before BLS signature verification in `validateExecutionPayloadBidGossip`. Duplicates of an already-seen bid now short-circuit without running `VerifySignature`.
