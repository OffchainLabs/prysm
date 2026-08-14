### Fixed

- Slab-allocate the validators returned by `BeaconState.ToProtoUnsafe` instead of allocating one protobuf validator (and two byte slices) per validator. This fixes a `SaveState` slowdown introduced with `CompactValidator`, where saving an archive point allocated ~4M objects for a mainnet-sized registry.
