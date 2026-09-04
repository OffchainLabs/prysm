### Fixed

- Fix Gloas checkpoint sync stalling and repeatedly downscoring peers when the checkpoint origin's payload is withheld. Preserve the origin's data columns when its payload is revealed so forward sync can process it.
