### Fixed

- Gloas: prime the engine for `current_slot+1` after processing a payload envelope, instead of `envelope.slot+1` which is stale when the payload arrives late.
