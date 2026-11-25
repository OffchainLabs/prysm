### Fixed

- Block production no longer deletes attestations from the pool when they could still be included in a later block, or when the requested slot is ahead of the clock. Found by the Anthropic OSS scanner.
