### Added

- Gloas builder circuit breaker now tracks which builder indices each direct connection endpoint serves. 
- New beacon flags `--builder-allowed-failures`, `--builder-critical-failures`, `--builder-blacklist-period`, `--builder-critical-blacklist-period`, `--builder-relay-blacklist-period`, `--builder-failure-backoff-period` and `--builder-critical-failed-builders`.
- New `--disable-builder-relay-circuit-breaker` flag to turn endpoint tracking off, and `builder_relays_banned_count` / `builder_collateral_blacklisted_count` metrics.
