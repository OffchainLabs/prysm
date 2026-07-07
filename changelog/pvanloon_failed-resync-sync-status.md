### Fixed

- A resync attempt that fails, or ends with the node still behind the wall clock, no longer re-marks the node as synced. Previously a node that fell behind and failed to resync would keep advertising itself as healthy via `/eth/v1/node/health` and `/healthz`, so load balancers kept routing validator duties to it while it served stale data. The regular sync service now also keeps retrying resync while the node remains behind, instead of never re-attempting after a failed resync.
