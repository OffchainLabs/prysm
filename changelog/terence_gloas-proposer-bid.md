### Added

- Pull execution payload bids from an external builder during Gloas block production, validate them against builder registry state and the proposer's max execution payment, select the best of self-build, P2P, and Builder-API bids, and submit the signed block to the builder when its bid wins.
