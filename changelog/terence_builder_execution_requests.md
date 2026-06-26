### Added

- Builder execution requests (EIP-8282): builders are onboarded and exited via `BuilderDepositRequest`/`BuilderExitRequest` in the Gloas `ExecutionRequests`, replacing the deposit-credential and voluntary-exit onboarding paths.

### Changed

- Update consensus spec tests to `v1.7.0-alpha.11`, aligning Gloas with the release: split `ExecutionRequests` into a Gloas-specific `ExecutionRequestsGloas` (with builder requests) leaving Electra/Fulu at three fields, add `BuilderPendingPayment.proposer_index`, validate execution payload bids against `state.slot`/`PAYLOAD_BUILDER_VERSION`, and set `MAX_BUILDER_DEPOSIT_REQUESTS_PER_PAYLOAD` to 256.
