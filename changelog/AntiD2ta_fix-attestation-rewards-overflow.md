### Fixed

- Fix uint64 overflow in attestation rewards endpoint epoch validation that allowed invalid future epoch requests to return data instead of 404 error (#15969).
