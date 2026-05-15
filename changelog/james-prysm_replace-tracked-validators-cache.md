### Added

- validators attached to this beacon node, are populated from `beacon_committee_subscriptions` and `prepare_beacon_proposer` for .
- Optional `validator_indices` field on the gRPC `CommitteeSubnetsSubscribeRequest`, fully used post gloas.

### Changed

- CGC and `validating()` source their attached-validator set from `SubscribedValidatorsCache` instead of `ProposerPreferencesCache`.
- `ProposerPreference.FeeRecipient` typed as `primitives.ExecutionAddress` instead of `[]byte`.
- FCU payload attribute builders fall back to `--suggested-fee-recipient` when no signed preference is cached.
- `prepare_beacon_proposer` (REST + gRPC) becomes a no-op one epoch before Gloas; Prysm VC skips the call from the same epoch.

### Removed

- `ProposerPreferencesCache.Validating()` and `Indices()`.
- `helpers.ProposerDependentRoot` wrapper.
