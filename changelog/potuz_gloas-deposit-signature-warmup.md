### Security

- Pre-verify pending deposit signatures in the two epochs before the Gloas fork and serve the
  results from a cache, so `onboard_builders_from_pending_deposits` does no BLS work at the fork
  boundary. Deposit signatures use a fork-agnostic domain, so a result is a pure function of the
  deposit and stays valid across the fork.
- Remove the quadratic `is_pending_validator` rescan from the Gloas fork upgrade. Onboarding now
  examines each accumulated pending deposit at most once instead of re-verifying the whole
  accumulator for every builder-credential deposit sharing a public key.

### Changed

- `builderInsertionIndex` no longer rescans the builder registry from index 0 on every insert
  during the Gloas fork upgrade, where no already-passed entry can become reusable.

### Added

- `ForEachPendingDeposit` reads the pending deposit queue without deep-copying it.
