### Added

- SSZ support for GET and POST of execution payload envelope and envelope contents.
- `broadcast_validation` query parameter on POST execution payload envelope.
- Spec-wire `WireBlindedExecutionPayloadEnvelope` types and `Eth-Execution-Payload-Blinded`
  header for the stateful publish path (beacon-APIs #580).
- `202` response on POST execution payload envelope when the envelope is broadcast
  but fails database integration (beacon-APIs #580).
- `ProduceBlockV4` returns only the beacon block when the produced block uses an
  external builder bid, regardless of `include_payload` (beacon-APIs #580).

### Changed

- `GET /eth/v1/validator/execution_payload_envelope/{slot}` →
  `GET /eth/v1/validator/execution_payload_envelopes/{slot}/{beacon_block_root}`;
  the response is the spec-wire `BlindedExecutionPayloadEnvelope` (payload replaced
  by `payload_root`, HTR equivalent to the full envelope). Returns only
  `Eth-Consensus-Version` (beacon-APIs #580 / PR #10).
- Stateful self-build now works end to end: the validator client fetches the blinded
  envelope from the BN, signs its (HTR-equivalent) root, and publishes the
  `SignedBlindedExecutionPayloadEnvelope`.
- `POST /eth/v1/beacon/execution_payload_envelopes` body shape is now selected by
  the required `Eth-Execution-Payload-Blinded` request header:
  - `true` → `SignedBlindedExecutionPayloadEnvelope` (stateful — BN reconstructs
    the full envelope from its cache).
  - `false` → `SignedExecutionPayloadEnvelopeContents` (stateless — body carries
    blobs and KZG proofs).
  Replaces the prior SSZ-lead-offset / JSON wrapper-key probe.
- Pluralized gloas execution payload endpoint paths to match the REST naming
  convention (beacon-APIs #613): `POST /eth/v1/beacon/execution_payload_bid` →
  `/eth/v1/beacon/execution_payload_bids`, and the execution payload envelope
  paths use `execution_payload_envelopes`.