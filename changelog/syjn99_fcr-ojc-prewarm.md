### Changed

- Fast confirmation rule: prewarm the next observed justified checkpoint balances on a background goroutine after the last slot's confirmation run, allowing the epoch-start lookup to reuse the cached balances.
