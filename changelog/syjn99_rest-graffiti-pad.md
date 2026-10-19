### Fixed

- REST validator client: pad or truncate the block graffiti to 32 bytes. Before, graffiti from proposer settings or the keymanager API that was not 32 bytes caused a 400 and a missed proposal.
