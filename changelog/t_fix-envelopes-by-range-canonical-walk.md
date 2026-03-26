### Changed

- Fix `ExecutionPayloadEnvelopesByRange` to only serve canonical payloads by walking the `ParentBlockHash` chain backward from a successor block.
- Add `ParentBlockHash` field to `BlindedExecutionPayloadEnvelope` proto to enable the backward walk without loading full blocks.
- Add block hash indexing for `BlindedExecutionPayloadEnvelope` in DB.
