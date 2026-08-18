### Fixed

- Beacon DB pruner: Keep the block and the state summary stored at the pruning cutoff. The cutoff is the last state-diff entry at or before the retention limit, and the states at or after it are rebuilt by replaying blocks from it, which needs its block root to stay resolvable. Deleting it made every state between the cutoff and the next state-diff entry unavailable, even though they are inside the retention period.
