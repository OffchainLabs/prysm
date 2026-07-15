### Fixed

- Remote web3signer keymanager: `NewKeymanager` now waits for the key-file watcher to initialize before returning and fails startup immediately if the watcher cannot be initialized, key-update notifications are sent without holding the keymanager lock, and keys are reloaded from the key file when the watcher recovers from a failure. This prevents a startup race, a potential lock-ordering deadlock, and a stale flag-only key set after watcher recovery.
