### Added

- Backfill Gloas execution payload envelopes: a new `batchSyncEnvelopes` stage fetches envelopes by range for revealed slots in each batch, verifies them statelessly (origin-state key snapshot with a `BUILDER_INDEX_SELF_BUILD` proposer-key path and a builder-registry `deposit_epoch` gate, block/bid binding, batched BLS over `DOMAIN_BEACON_BUILDER`, and a locally computed execution block hash checked against the bid, with no EL dependency), and persists them blinded alongside the blocks.
- Metrics: `backfill_envelopes_download_count`, `backfill_envelopes_verified_count`, `backfill_envelope_slots_skipped` (by reason: `sig_unverifiable`, `peer_exhausted`, `tail_unresolved`, `db_failed`), and `backfill_envelope_conflicts_kept`.
