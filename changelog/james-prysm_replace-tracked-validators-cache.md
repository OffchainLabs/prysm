### Added

- `SubscribedValidatorsCache` tracks attached validators via `beacon_committee_subscriptions` for CGC and `validating()` computation, but has prepare_beacon_proposer populating in case of using an old validator client.
- `validator_indices` field on `CommitteeSubnetsSubscribeRequest` (gRPC): a flat list of attached validators, independent of the (slot, committee) subnet dedup, so the BN cache tracks every attached validator.

### Changed

- CGC and `validating()` source their attached-validator set from `SubscribedValidatorsCache` instead of `ProposerPreferencesCache`.
- FCU payload attribute builders fall back to `--suggested-fee-recipient` when no signed preference is cached.
- `prepare_beacon_proposer` (REST + gRPC) is a no-op post-Gloas; the VC stops calling it post-Gloas. `SignedProposerPreferences` replaces it.
- REST `beacon_committee_subscriptions` sends one `BeaconCommitteeSubscription` per active validator (no longer deduped by subnet) so the BN cache tracks every attached validator.

### Removed

- tracked validator cache and prefer to use proposer preferences cache and subscribed validators cache.
