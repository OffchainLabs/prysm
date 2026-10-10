### Fixed

- Bound the signature checks a gossip peer can trigger on the `execution_payload` topic with a per-peer, per-slot budget, so a peer cannot force unlimited payload hashing by sending envelopes with invalid signatures.
