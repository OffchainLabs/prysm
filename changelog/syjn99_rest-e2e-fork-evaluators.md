### Changed

- E2E fork transition evaluators now poll `GET /eth/v2/beacon/blocks/head` over the beacon API instead of opening a gRPC `StreamBlocksAltair` stream, and share a single helper across all six forks.

### Removed

- Removed the validator client's `StreamBlocksAltair` implementations (`beacon-api` polling shim and `grpc-api` passthrough). The method was not part of `validator/client/iface` and had no callers.
