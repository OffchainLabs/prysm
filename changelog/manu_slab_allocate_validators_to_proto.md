### Fixed

- Slab-allocate the validators returned by `BeaconState.ToProtoUnsafe` instead of allocating one protobuf validator (and two byte slices) per validator.