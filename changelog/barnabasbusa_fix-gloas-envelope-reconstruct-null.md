### Fixed

- Handle `null` responses from `eth_getBlockByHash` in the Gloas execution payload envelope reconstruction path. Previously the `GET /eth/v1/beacon/execution_payload_envelope/{block_id}` handler returned HTTP 500 with `missing required field 'parentHash' for Header` when the EL had not yet executed a payload the CL already had a blinded envelope for; it now returns HTTP 425 Too Early with a `Retry-After` hint.
