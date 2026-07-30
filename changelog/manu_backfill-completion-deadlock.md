### Fixed

- Backfill: fix a deadlock where a node that backfilled all the way down to its retention window would hang instead of reporting completion.