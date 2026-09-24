### Added

- Progressively lower the persisted and advertised `earliest_available_slot` to the imported low slot after each durably committed backfill batch, so Status v2 reflects backfilled history as it becomes available (partial progress on #15982).
- Conservatively skip `earliest_available_slot` lowering during backfill when the custody group count changed since backfill started, or when the advertised value was already raised above the backfill origin.
