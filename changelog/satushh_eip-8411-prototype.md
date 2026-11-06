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

### Fixed

- Order `parent_beacon_block_root` before `builder_index` in `ExecutionPayloadEnvelopeHeze` to match the EIP-8411 container definition. The previous order changed the SSZ serialization and therefore `payload_chunks_root`, so chunks would not have interoperated with other clients.
- Forward a buffered payload chunk once its bid commitment becomes known. Chunks arriving before their commitment are returned as `ValidationIgnore`, so libp2p never relays them; the drain path previously fed them only into the local reconstruction pool, leaving peers that depend on this node without the chunk.
- Clear the first-seen markers of chunks discarded by payload-chunk pool eviction. The bytes were dropped while the markers remained, so the evicted chunks were refused when re-offered and that commitment could never be reconstructed again.
- Keep one slot of grace when pruning payload-chunk buffers. Gossip validation admits a previous-slot chunk within `MAXIMUM_GOSSIP_CLOCK_DISPARITY`, so pruning exactly at the slot boundary discarded partial sets whose remaining chunks were still admissible.
- Set an explicit gossip subscription buffer of 512 messages. libp2p's default is 32 and it **drops** rather than blocks on overflow, so an EIP-8411 envelope published as 64 back-to-back chunks silently lost part of every burst after validation had already accepted it. Measured on a devnet: 2148 undeliverable chunks and 4069 stranded in sets that could never complete, with only 66% of payloads reaching every peer; with the buffer raised, both counters are zero and completeness is 99%.
- Enforce `MAX_PAYLOAD_CHUNK_SIZE` when verifying a payload chunk's inclusion proof. The generated `PayloadChunk.UnmarshalSSZ` applies no upper bound to the chunk byte list (only `MarshalSSZ` does) and the Merkle leaf is `sha256(chunk)` over whatever bytes arrived, so an oversized chunk both decoded and proof-verified. A builder could therefore commit to a chunk set whose pieces each exceed the bound and have every node accept and buffer `TOTAL_EXECUTION_PAYLOAD_CHUNKS` of them.
- Claim the first-seen `(payload_chunks_root, index)` marker atomically via `slotAwareCache.AddIfAbsent`. Gossip validation runs concurrently per topic, so the previous has-then-set pair let two deliveries of the same chunk both observe "not seen" and both accept, forward and insert it.
- Re-run the slot-window rule when draining buffered payload chunks. A chunk can sit in the pending buffer across a slot boundary and the drain path forwards to peers, so without this a node could relay a chunk it would itself reject on arrival.

