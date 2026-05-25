### Fixed

- Pass the forwarding peer ID into `queuePendingGloasColumn` so deferred Gloas data column failures actually downscore the sender. `msg.GetFrom()` is empty under Prysm's `StrictNoSign`/`WithNoAuthor` pubsub config, which made the bad-response increment a no-op.
