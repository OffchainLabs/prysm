### Changed

- Do not walk all validators with `ExitInformation` for Electra and later blocks with voluntary exits or slashings. Slashings now read the total active balance from the cache.
