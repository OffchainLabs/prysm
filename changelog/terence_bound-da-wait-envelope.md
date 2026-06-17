### Fixed

- Bound the data column availability wait in `ReceiveExecutionPayloadEnvelope` so an envelope with unavailable columns no longer blocks the import path indefinitely.
