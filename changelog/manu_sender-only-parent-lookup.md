### Fixed

- Request a missing block from the best peers if it is an ancestor of a block from the proposer expected by the head state. Otherwise, request it only from the peer that referenced it, and downscore that peer if it does not serve it.