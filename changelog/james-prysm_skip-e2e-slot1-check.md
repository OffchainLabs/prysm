### Changed

- e2e sync committee evaluator now skips the first slot after startup, we already skip the fork epoch for checks here, this skip only applies on startup, due to altair always from 0 and validators need to warm up.