### Changed

- Compute per-element hash tree roots in parallel for large SSZ lists, and read the feature config atomically instead of under a mutex.
