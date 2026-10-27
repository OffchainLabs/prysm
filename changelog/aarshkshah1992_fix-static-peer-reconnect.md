### Fixed

- Close an outbound connection that was skipped because the peer disconnected less than a second ago.
- `GET /eth/v1/node/peers/{peer_id}` no longer returns a 500 for peers without an ENR, such as static peers.