### Added

- Consolidation request metrics in blockchain processing.
- Overlay mode for `FieldTrie` enabling copy-on-write state sharing: overlays store only modified nodes against an immutable, ref-counted base trie, avoiding full trie copies on `state.Copy`.

### Changed

- State fields now initialize field tries only where needed, reducing unnecessary overhead.
- `FieldTrie` rewritten to support two modes: owned (flat contiguous buffer) and overlay (sparse diffs against a base trie).
