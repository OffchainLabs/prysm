### Fixed

- Mark a gossip execution payload bid as seen only after all checks pass, as the spec does. A bid that fails a transient `[IGNORE]` check can now be re-validated.
