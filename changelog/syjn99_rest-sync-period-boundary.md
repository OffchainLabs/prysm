### Fixed

- REST validator client: request sync committee duties for the epoch of `slot + 1`, so that the last slot of a sync committee period uses the next committee, the same as gRPC.
