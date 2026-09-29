### Fixed

- Persist the execution payload envelope before emitting the `execution_payload_available` event so that `GET /eth/v1/beacon/execution_payload_envelopes/{block_id}` no longer returns 404 for consumers reacting to the event; the envelope is removed again if execution validation fails.
