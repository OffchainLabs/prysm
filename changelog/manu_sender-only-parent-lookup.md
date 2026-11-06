### Fixed

- Request a missing block only from the peer that referenced it (the gossip sender, or the peer serving its child), once, and downscore that peer if it does not serve it.
