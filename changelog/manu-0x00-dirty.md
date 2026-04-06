### Added

- Prometheus summary metric (`field_trie_recompute_indices`) tracking the number of changed indices per `RecomputeTrie` call, broken down by field.

### Fixed

- `ProcessEffectiveBalanceUpdates`: avoid copying a validator when the computed effective balance is unchanged, reducing unnecessary allocations.
