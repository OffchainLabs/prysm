### Changed

- Compute every slot- and epoch-based duration from `SLOT_DURATION_MS` instead of `SECONDS_PER_SLOT`, via `SlotDuration()`, `SlotDurationMillis()`, `SlotsDuration()` and `EpochsDuration()`. `SECONDS_PER_SLOT` is no longer read outside `config/params`, so a chain config expressing its slot duration in milliseconds no longer has to be a whole number of seconds. `ConfigToYaml` keeps emitting the legacy `SECONDS_PER_SLOT` key for consumers that predate `SLOT_DURATION_MS`, but omits it when it cannot represent the configured duration.
