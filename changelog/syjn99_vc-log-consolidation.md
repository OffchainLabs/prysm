### Changed

- Consolidated validator client submission logs: sync committee messages, sync contributions, and payload attestations are now summarized as one info line per distinct submission content per slot (with range-compressed validator indices), and the previous per-validator success logs were downgraded to debug level.
