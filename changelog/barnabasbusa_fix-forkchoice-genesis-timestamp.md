### Fixed

- Forkchoice no longer logs `could not check if block arrived early: invalid timestamp` on beacon nodes that start before `MIN_GENESIS_TIME`. The tree root node (genesis or checkpoint-sync finalized block) is never a reorg candidate, so `arrivedEarly` and `arrivedAfterOrphanCheck` now short-circuit for it instead of comparing its process-startup insertion timestamp against the slot start.
