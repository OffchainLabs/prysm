### Fixed

- Fixed a race in the state-diff anchor cache where a reader could memoize a replaced anchor, so later diffs were computed against a stale anchor.
