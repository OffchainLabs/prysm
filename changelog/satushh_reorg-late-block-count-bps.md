### Fixed

- Derive the late-slot `UpdateHead` tick, its clock disparity, and the `ShouldOverrideFCU` threshold (`ProcessAttestationsThreshold`) from `PROPOSER_REORG_CUTOFF_BPS` instead of hardcoded 2s and 10s values, so late-block reorg timing scales with the slot duration; an unusable cutoff disables the late tick and proposer reorg decisions.
