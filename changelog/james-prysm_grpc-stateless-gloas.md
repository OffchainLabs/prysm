### Added

- gRPC support for the `--stateless` gloas self-build path (previously REST-only), serving both
  stateless (contents) and stateful (blinded) envelope publishing via a
  `GenericSignedExecutionPayloadEnvelope` oneof.
