### Fixed

- Bound the signature hashing a gossip peer can trigger on the `execution_payload` topic. A peer can send distinct envelopes with invalid signatures and make the node hash their full payloads. Failed and in-flight checks now share a per-peer, per-slot budget. Unknown-block checks have a separate budget because they use the head state, which may differ from the block's state; their failures cannot prevent later verification once the block is known.
- Reject a gossip `SignedExecutionPayloadEnvelope` whose `parent_beacon_block_root` does not match the parent root of its beacon block, instead of discovering the mismatch during the state transition.
