### Changed
- Backfill and range syncing cooperatively share an exclusive "lock" over RPC access. Range syncing will hold the lock for an entire round robin sync cycle, while backfill maintains more coarse grained locks on individual units of work, in order to prioritize initial-sync.
