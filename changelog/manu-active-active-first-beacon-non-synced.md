### Fixed

- REST validator client with multiple beacon nodes: wait for sync no longer blocks when the first beacon node is syncing and another one is synced.
- REST validator client with multiple beacon nodes: the doppelganger check no longer fails when the first beacon node is syncing and another one is synced.
- REST validator client with multiple beacon nodes: aggregation no longer fails when the first beacon node is optimistic and another one is synced and not optimistic.
- REST validator client with multiple beacon nodes: the doppelganger check uses the head of a synced beacon node instead of the head of the first beacon node.
