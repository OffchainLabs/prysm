### Fixed

- adding in improvements to getduties v2, calling helpers.PrecomputeCommittees() might be slow if used for not a lot of keys, so this implements hybrid strategy based on validatorLookupThreshold (3000)
  - < 3000 validators: Uses helpers.CommitteeAssignments() - only computes for requested validators
  - ≥ 3000 validators: Uses helpers.PrecomputeCommittees() + map for O(1) lookups