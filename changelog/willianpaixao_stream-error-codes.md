### Changed

- Upgrade go-libp2p from v0.39.1 to v0.45.0.
- Replace `stream.Reset()` with `stream.ResetWithError(code)` for better error signaling to peers.

### Fixed

- Fix `NoError` test assertion panic on struct error types by using `deepNil()`.
- Fix multiaddr comparison in tests using `.Equal()` method for go-multiaddr v0.16.0 compatibility.
