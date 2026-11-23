### Fixed

- Feed gossip-validated aggregate attestations to the slasher when it is enabled. Send an aggregate only if it has an attesting bit that no earlier aggregate with the same data and committee had.
