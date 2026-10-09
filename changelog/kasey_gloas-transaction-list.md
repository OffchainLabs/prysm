### Added
- Custom types for more compact representation (marshaled ssz byte slice) of ExecutionPayload.Transactions.
- `GetExecutionPayloadEnvelopeV2` and `PublishExecutionPayloadEnvelopeV2` gRPC endpoints using these new types.

### Changed

- `ExecutionPayloadGloas.transactions` is now a `ProgressiveTransactionList` under a new protobuf field tag; tag 14 is reserved. Any external code that imports ExecutionPayload and interacts with the Transactions field expecting a [][]byte will need to be updated.
- Note: Validator clients and beacon nodes must be upgraded together on networks that have forked into Gloas networks; pre-Gloas networks are unaffected. The V1 `GetExecutionPayloadEnvelope` and `PublishExecutionPayloadEnvelope` gRPC endpoints now return `FAILED_PRECONDITION`. A validator client that still calls them predates the new encoding and would otherwise sign envelopes with an empty transaction list. 
