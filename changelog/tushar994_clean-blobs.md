### Fixed

- Prune stale pre-Fulu blobs on startup. After the Fulu fork no new blobs are ever saved, so the save-driven pruning path never fires. `WarmCache` now runs a single prune pass using the current network epoch at startup, ensuring all blobs that have aged past the retention window are deleted even if they were never pruned during normal operation.
