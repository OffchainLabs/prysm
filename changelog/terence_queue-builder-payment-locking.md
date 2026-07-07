### Fixed

- Hold the state lock in `QueueBuilderPaymentForSlot` and route builder pending withdrawal appends through one copy-on-write path.
