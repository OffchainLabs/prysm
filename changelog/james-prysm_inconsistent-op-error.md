### Fixed

- Fixed inconsistencies around endpoints that use a helpers.IsOptimistic which returned a 500 error for state not found when it should have returned a 404 error.