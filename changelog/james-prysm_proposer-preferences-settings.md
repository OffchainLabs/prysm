### Changed

- proposer settings no longer recognize the builder option post gloas and introduces a new gas_limit option for proposer preferences, supported per validator and in the default config.
- the builder gas_limit and the new top-level gas_limit are independent signals: relay registrations keep reading builder.gas_limit pre-gloas, while proposer preferences read only the top-level gas_limit (falling back to the default config, then the chain default).
- gas limit keymanager endpoint continues to update gas limit on a per validator basis post gloas.
- v1 proposer settings files remain supported without modification: a deprecation warning is logged on gloas-scheduled networks and settings are upgraded automatically in the validator client at the gloas fork, promoting builder gas limits to the top level unless one is already set. Settings already on v2 are never rewritten.
