### Fixed

- Release the RPC rate limiter's lock before replying to a rate-limited peer, and set a write deadline on that reply.
