### Added

- Validator client: keys added through a keymanager reload are now held out of duties until a doppelganger check clears them (behind `--enable-doppelganger`).

### Fixed

- Validator client: keys the beacon node cannot evaluate yet (e.g. deposits pending) no longer block startup or pass unchecked; they are held out of duties and re-checked once per epoch until they can be evaluated (behind `--enable-doppelganger`).
- Validator client: an epoch with no eligible validating keys no longer leaves the duty schedule uninitialized, which previously produced a role-lookup error log every slot.
- Validator client (REST): a doppelganger check whose keys are all absent from the beacon state now returns cleanly instead of failing on the liveness request, matching the gRPC path (behind `--enable-doppelganger`).
