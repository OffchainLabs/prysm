### Fixed

- Use `HeadSlot()` in `computePayloadWithdrawals` instead of reading `s.head.slot` directly, to avoid a data race with `setHead`.
