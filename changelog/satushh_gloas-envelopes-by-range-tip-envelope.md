### Fixed

- Serve the chain tip's execution payload envelope in `ExecutionPayloadEnvelopesByRange` responses when fork choice selects the tip's full payload variant, per the `PAYLOAD_STATUS_FULL` head condition of consensus-specs#5608.
