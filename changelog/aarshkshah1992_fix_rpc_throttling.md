### Added

- Per-peer bandwidth throttling of rpc responses: each peer's responses (blocks, blobs, data columns, envelopes, light client data) are served through a shared per-peer token bucket, with a bounded number of concurrent response streams per peer. Requests on the peering topics (status, ping, metadata, goodbye) are never throttled. Throttling never records a bad response against the peer: a peer whose responses are slowed, or whose request could not obtain a response stream in time, is not penalized.
- New flags `--rpc-throttle-bps` (default 1 MiB/s, `0` disables throttling), `--rpc-throttle-burst` (default streams-per-peer × bps, 4 MiB) and `--rpc-throttle-streams-per-peer` (default 4) to configure the rpc response throttle, validated at startup.
- New Prometheus metrics `rpc_throttle_written_bytes`, `rpc_throttle_stream_milliseconds`, `rpc_throttle_wait_failed_total`, `rpc_peer_throttle_create_total` and `rpc_peer_throttle_prune_total` for observing rpc response throttling.
