### Fixed

- Validator client: keys added through a keymanager reload are now held out of duties until a doppelganger check clears them (behind `--enable-doppelganger`).
- Validator client: the startup doppelganger check no longer prevents startup when the validating keys are not yet known to the beacon node.
