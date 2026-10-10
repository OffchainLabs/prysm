### Changed

- Replace the per-fork state constructors `state_native.InitializeFromProto[Unsafe]<Fork>` with the generic `state_native.New` and `state_native.NewUnsafe`. Replace the per-fork `state_native.ProtobufBeaconState<Fork>` extractors with the generic `state_native.ContainerFrom`.
