### Fixed

- Reconcile Web3Signer key-file changes by public-key set so equal-count edits and editor save bursts update validator accounts exactly once.
- Persist validator key and account files with atomic replacement (following symlinks and normalizing permissions to 0600), keep file watchers across replacements without exhausting kqueue descriptors, and retry absent, empty, or invalid reloads without discarding the last valid key set.
