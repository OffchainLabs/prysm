### Fixed

- Return the confirmed block's payload hash as the safe execution block hash even when it is zero (pre-merge, genesis in spec tests) instead of falling back to the unrealized justified hash.
- Gloas: when no full ancestor is left in forkchoice, resolve the payload a block builds on to the tree root's bid `parent_block_hash` instead of zero.

### Ignored

- Revert the spectest-only stub pubkey aggregation (`SetStubPubkeyAggregation`): consensus-spec-tests `>= v1.7.0-alpha.13` compute real aggregate pubkeys with BLS disabled (consensus-specs#5489), so the stub breaks `next_sync_committee.aggregate_pubkey` on newer vectors.
