### Changed

- `validator accounts voluntary-exit` now fetches genesis through the validator client factory instead of dialing gRPC directly, so `--enable-beacon-rest-api` and `--beacon-rest-api-provider` are honored when a remote signer is used.
