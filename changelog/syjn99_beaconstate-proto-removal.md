### Changed

- `BeaconState*` (Phase0 through Gloas), `HistoricalBatch`, `SigningData`, `ForkData`, `DepositMessage` and `PowBlock` in `proto/prysm/v1alpha1` are now hand-written SSZ structs and no longer implement `proto.Message`. Use `.Copy()` and `equality.DeepEqual` instead of `proto.Clone` and `proto.Equal`. On-disk and wire formats are unchanged.
- `StateSummary`, `PendingAttestation` and `SyncAggregatorSelectionData` moved out of the deleted `beacon_state.proto` into `beacon_core_types.proto`, `attestation.proto` and `sync_committee.proto` (same Go package, field numbers and wire format).

### Removed

- `proto/prysm/v1alpha1/beacon_state.proto` and the unused `CheckPtInfo` message.
