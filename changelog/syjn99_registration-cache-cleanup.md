### Changed

- `GetFeeRecipientByPubKey` now reads the proposer preferences cache instead of the beacon DB. `PrepareBeaconProposer` has written to that cache rather than the DB for some time, so the endpoint returned the default fee recipient on any node that never ran an older Prysm.

### Deprecated

- `--disable-registration-cache` is deprecated and now has no effect. The validator registration cache is always used; registrations are no longer persisted to the beacon DB.

### Removed

- Removed the unreachable validator registration and fee recipient beacon DB paths: `RegistrationByValidatorID`, `SaveRegistrationsByValidatorIDs`, `FeeRecipientByValidatorID`, `SaveFeeRecipientsByValidatorIDs`, and the `registration` and `fee-recipient` buckets.
