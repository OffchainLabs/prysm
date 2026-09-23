### Changed

- PTC data reads share one bounded retry policy across gRPC and REST; REST beacon nodes are polled independently so stalled nodes do not delay responsive peers.
