### Fixed

- Reconcile Web3Signer key-file changes by public-key set so equal-count edits and editor save bursts update validator accounts exactly once.
- Persist validator key and account files with atomic replacement, keep file watchers across replacements, and retry invalid or transient reloads without discarding the last valid key set.
