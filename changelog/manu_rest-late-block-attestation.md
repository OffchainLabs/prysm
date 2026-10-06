### Fixed

- REST validator client: no longer drop an attestation or a sync committee message when no beacon node answers before the attestation due time. A slow answer is now used instead of no answer at all.
