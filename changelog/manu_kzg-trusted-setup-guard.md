### Fixed

- Only mark the KZG trusted setup as loaded once loading it actually succeeded.
  The guard was previously set beforehand, so a failed load left it disagreeing
  with the equivalent flag c-kzg keeps internally, and any later attempt to load
  the setup was skipped instead of retried. No user-visible impact is known,
  since a failing load already prevents the beacon node from starting.
