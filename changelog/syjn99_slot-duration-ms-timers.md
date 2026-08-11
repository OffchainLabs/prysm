### Fixed

- Derive the remaining slot- and epoch-length timers from `SLOT_DURATION_MS` instead of the raw `SECONDS_PER_SLOT` field. Broadcast and RPC timeouts, subnet/attestation cache TTLs, peer status and resync intervals, the pending-block queue deadline and the attestation pruning interval all computed their durations from `SecondsPerSlot`, which a config that only sets `SLOT_DURATION_MS` leaves at its preset default. They now run at the configured slot rate. The `SECONDS_PER_SLOT` config key itself is unchanged and still backs `SlotDuration()` when `SLOT_DURATION_MS` is absent.
