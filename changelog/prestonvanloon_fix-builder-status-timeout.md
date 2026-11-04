### Fixed

- Add a 10 second timeout to the builder (MEV relay) status checks made at start-up and while polling, so an unresponsive relay can no longer block the beacon node from starting. Discovered by Anthropic's OSS Scanner.
