### Fixed

- Raise global per-peer RPC rate limit (5→50 req/s, 10→100 burst) and stop downscoring peers that exceed it; rate-limit responses no longer accumulate toward the bad-peer threshold.
