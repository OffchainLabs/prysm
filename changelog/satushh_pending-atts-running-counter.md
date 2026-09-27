### Changed

- Track the number of queued pending attestations with a running counter instead of summing every entry of `blkRootToPendingAtts` under `pendingAttsLock` on each insert, making the `pendingAttsLimit` check O(1).
