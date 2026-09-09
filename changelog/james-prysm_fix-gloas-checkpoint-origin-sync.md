### Fixed

- Fix Gloas checkpoint sync when the origin payload is withheld. Defer origin sidecar fetching to forward sync, preserve required FULL-parent data availability, and accept origin envelopes against checkpoint states advanced through skipped slots.
