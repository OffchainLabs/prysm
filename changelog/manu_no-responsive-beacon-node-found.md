### Changed

- Validator client: when no beacon node is ready, log why (unreachable, syncing, or optimistic because the execution client is not synced) instead of always logging "No responsive beacon node found".

### Fixed

- The gRPC health endpoint reports a synced but optimistic beacon node as "node is optimistic" instead of "node is syncing".
