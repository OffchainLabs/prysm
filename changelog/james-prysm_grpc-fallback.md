### Changed

- gRPC fallback now matches rest api implementation and will also check and connect to only synced nodes.

### Removed

- gRPC resolver for load balancing, the new implementation matches rest api's so we should remove the resolver so it's handled the same way for consistency.