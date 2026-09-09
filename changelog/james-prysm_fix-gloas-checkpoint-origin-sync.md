### Fixed

- Fix Gloas checkpoint sync when the origin payload is withheld. Recover missing FULL-parent envelopes by root during forward sync, preserve and check their data columns, and accept origin envelopes against checkpoint states advanced through skipped slots.
