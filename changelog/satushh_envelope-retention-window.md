### Added

- `Env` retention span in `das.CurrentNeeds` bounding envelope backfill to `max(gloasStart, MIN_EPOCHS_FOR_BLOCK_REQUESTS floor)`; it never inherits `--backfill-oldest-slot`.
