### Added

- gRPC support for the `--stateless` gloas self-build path (previously REST-only). Block production
  returns the bundled execution payload envelope + blobs in a single call via a new `gloas_contents`
  variant on `GenericBeaconBlock`.
- `GenericSignedExecutionPayloadEnvelope` oneof (contents | blinded) for the
  `PublishExecutionPayloadEnvelope` RPC, mirroring `GenericSignedBeaconBlock`, so one endpoint serves
  both stateless (contents) and stateful (blinded) publishing.

### Changed

- gRPC envelope publishing is now fully symmetric with REST: stateful self-build publishes the
  blinded envelope (the beacon node reconstructs the full payload from its cache) and stateless
  publishes the full contents + blobs. `GetExecutionPayloadEnvelope` now returns the blinded wire
  form, and the validator client validates its `beacon_block_root` against the proposed block.
- The blinded-from-full reconstruction is centralized in the v1alpha1 server, shared by the REST and
  gRPC publish paths; the `WireBlindedFromFull`/`SignedWireBlindedFromFull` conversions moved from
  `api/server/structs` to the core `consensus-types/blocks` package.
- Both validator clients share a single `validator/client/cache` package (the VC-side analog of
  `beacon-chain/cache`) for the produce → publish envelope handoff, and a single backend-agnostic
  `iface.Option` / `iface.WithStateless` for client construction.
