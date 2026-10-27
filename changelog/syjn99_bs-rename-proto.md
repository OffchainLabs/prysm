### Changed

- Rename `BeaconState.ToProto` and `BeaconState.ToProtoUnsafe` to `ToContainer` and `ToContainerUnsafe`. They return the hand-written state container, not a protobuf message.
