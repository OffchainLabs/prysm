### Changed

- proposer settings no longer recognize the builder option post gloas and introduces a new gas_limit option for proposer preferences, supported per validator and in the default config.
- gas limit keymanager endpoint continues to update gas limit on a per validator basis post gloas.
- v1 proposer settings files remain supported without modification: a deprecation warning is logged on gloas-scheduled networks and settings are upgraded automatically in the validator client at the gloas fork.
