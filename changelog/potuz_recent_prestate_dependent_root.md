### Fixed
- Use the target epoch's shuffling dependent root to decide whether the head state can validate an attestation, so previous-epoch and next-epoch attestations on a compatible branch no longer fall back to state regeneration.
