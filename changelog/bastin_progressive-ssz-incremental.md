### Added

- Cache the progressive Merkle tree for the Gloas beacon state and its
  validator/balance field tries, so a hash tree root after a small mutation
  recomputes only the affected subtrees.
