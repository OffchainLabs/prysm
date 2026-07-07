### Changed

- Updated `go-libp2p-pubsub` to latest release, which allows republishing a previously seen but never delivered message (e.g. an attestation ignored because its block had not arrived yet). Gossipsub now reruns validation on such publishes and forwards them, so pending attestations are propagated once their block is imported instead of being silently dropped.
