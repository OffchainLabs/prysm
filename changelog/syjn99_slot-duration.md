### Fixed

- Derive the remaining slot- and epoch-length timers from `SLOT_DURATION_MS` instead of the raw `SECONDS_PER_SLOT` field.
- Drive slot tickers from `SLOT_DURATION_MS` instead of the raw `SECONDS_PER_SLOT` field.
