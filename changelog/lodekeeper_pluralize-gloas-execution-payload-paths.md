### Changed

- Pluralized gloas execution payload endpoint paths to match the REST naming
  convention used elsewhere in the beacon-API spec (beacon-APIs #613):
  - `POST /eth/v1/beacon/execution_payload_bid` → `POST /eth/v1/beacon/execution_payload_bids`
  - `GET /eth/v1/beacon/execution_payload_envelope/{block_id}` → `GET /eth/v1/beacon/execution_payload_envelopes/{block_id}`
