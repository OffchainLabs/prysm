### Changed

- Implement defer payload processing spec change (consensus-specs#5094): move execution payload state mutations to `process_parent_execution_payload` in the next block, make `process_execution_payload` pure verification, add `execution_requests_root` to `ExecutionPayloadBid`, remove `state_root` from `ExecutionPayloadEnvelope`, and add `parent_execution_requests` to `BeaconBlockBody`.
