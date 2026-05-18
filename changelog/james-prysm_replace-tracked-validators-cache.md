### Added

- `SubscribedValidatorsCache` tracks attached validators via `beacon_committee_subscriptions` for CGC and `validating()` computation.
- `validator_indices` field on `CommitteeSubnetsSubscribeRequest` (gRPC): a flat list of attached validators, independent of the (slot, committee) subnet dedup, so the BN cache tracks every attached validator.

### Changed

- CGC and `validating()` source their attached-validator set from `SubscribedValidatorsCache` instead of `ProposerPreferencesCache`.
- `ProposerPreference.FeeRecipient` typed as `primitives.ExecutionAddress` instead of `[]byte`.
- FCU payload attribute builders fall back to `--suggested-fee-recipient` when no signed preference is cached.
- `prepare_beacon_proposer` (REST + gRPC) is a no-op post-Gloas; the VC stops calling it post-Gloas. `SignedProposerPreferences` replaces it.
- REST `beacon_committee_subscriptions` sends one `BeaconCommitteeSubscription` per active validator (no longer deduped by subnet) so the BN cache tracks every attached validator.

### Removed

- `ProposerPreferencesCache.Validating()` and `Indices()`.
- `helpers.ProposerDependentRoot` wrapper.
