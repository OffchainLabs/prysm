### Added

- Add `execution.WithRPCClientDialer` option allowing an embedding process to supply the execution node RPC client (e.g. one backed by `rpc.DialInProc`) instead of dialing the configured HTTP endpoint.

### Fixed

- Attach the execution service's RPC client only after chain ID validation succeeds, so a failed reconnection attempt no longer replaces a working client with a closed one or leaks the previous client.
