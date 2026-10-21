### Added

- Prototype EIP-8411 (fast execution payload broadcast), gated at the Heze fork: the Heze containers live in `heze.proto` (`ExecutionPayloadBidHeze` commits to the chunked envelope via `payload_chunks_root`, `ExecutionPayloadEnvelopeHeze` drops `beacon_block_root`), envelopes travel as 64 `PayloadChunk` messages on the `execution_payload_chunks` topic (registered from `HezeForkEpoch`) with per-chunk Merkle inclusion proofs, and nodes reconstruct, strictly decode, and import the envelope through the existing pipeline. Chunk machinery lives in `beacon-chain/core/heze`.
- Envelopes reconstructed from verified chunks carry a zero sentinel signature and are authenticated against the bid's `payload_chunks_root` commitment (`heze.VerifyEnvelopeChunkCommitment`).
- `GetExecutionPayloadEnvelopeV3`/`PublishExecutionPayloadEnvelopeV3` validator RPCs and `SignRequest` arm `execution_payload_envelope_heze` for the Heze envelope flow; the REST envelope endpoints serve and publish the Heze container for Heze slots via the `heze` version header, with the beacon block root carried out of band.
- Envelope by-root/by-range req/resp serve the slot's fork-correct wire container with per-slot fork-digest context bytes; initial-sync batches never mix Gloas and Heze envelope protocols.
- Metrics `sync_payload_chunk_arrival_delay_seconds`, `sync_payload_chunk_reconstruction_seconds`, `sync_payload_chunk_received_total`, and `sync_payload_chunk_reconstruction_total`.
- Metrics `sync_payload_envelope_usable_delay_seconds` and `sync_payload_envelope_size_bytes`, labeled by `transport` (`gossip` for the monolithic envelope message, `chunks` for EIP-8411 reconstruction), measuring slot start to full local envelope availability and the envelope size at that point.

### Changed

- `Body().SignedExecutionPayloadBid()` returns a read-only bid wrapper (`interfaces.ROSignedExecutionPayloadBid`) so Gloas and Heze bid containers share one consumer surface; `BeaconStateHeze` carries the Heze bid with `payload_chunks_root`.
- Heze blocks are self-build only for now: external bid paths are skipped at Heze and the self-build bid commits to the bid-time envelope's chunk root.

### Removed

- The `execution_payload` gossip topic retires at `HezeForkEpoch` (Gloas keeps it; its validator and subscriber are unchanged pre-Heze).
