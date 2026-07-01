### Fixed

- Skip peers returning empty `ExecutionPayloadEnvelopesByRange` responses when the block bids prove payloads exist; downscore only when the same peer served both the blocks and the empty response.
