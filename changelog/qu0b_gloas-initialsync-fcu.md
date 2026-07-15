### Fixed

- Keep the execution client supplied with a forkchoice target during Gloas initial-sync: re-advertise the current head payload (already in forkchoice) when a batch import fails, and notify the head payload from the gossip envelope path while syncing.
