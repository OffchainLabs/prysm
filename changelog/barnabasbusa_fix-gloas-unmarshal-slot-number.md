### Fixed

- Fixed `ExecutionBundleGloas.UnmarshalJSON` dropping the EIP-7843 `slotNumber` field returned by `engine_getPayloadV6`. Self-build execution payload envelopes were cached with `SlotNumber=0`, causing `GetExecutionPayloadEnvelope(slot)` to return `NotFound` and stalling the execution chain at the Gloas fork boundary.
