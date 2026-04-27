### Added

- Validator client flag `--enable-stateless` (alias `--stateless`) for the
  Gloas stateless block production path. The validator requests the block,
  execution payload envelope, blobs, and KZG proofs in a single
  `/eth/v4/validator/blocks` call (`include_payload=true`), then publishes them
  back via `POST /eth/v1/beacon/execution_payload_envelope` using the
  `SignedExecutionPayloadEnvelopeContents` body so the receiving beacon node
  does not need to have the data cached locally.

### Changed

- `GET /eth/v4/validator/blocks/{slot}` now populates `kzg_proofs` and `blobs`
  in the `BlockContentsGloas` response (JSON and SSZ) when `include_payload=true`.
- `POST /eth/v1/beacon/execution_payload_envelope` now accepts either
  `SignedExecutionPayloadEnvelope` (stateful) or
  `SignedExecutionPayloadEnvelopeContents` (stateless). When Contents are
  supplied the beacon node builds Gloas data column sidecars on the fly from
  the supplied blobs and KZG proofs and broadcasts them alongside the envelope.
