### Ignored

- Remove the unused `ValidatorParticipation` and `ValidatorQueue` methods from the validator's `ChainClient` interface and its gRPC/REST implementations. The beacon node keeps serving both endpoints for external consumers.
