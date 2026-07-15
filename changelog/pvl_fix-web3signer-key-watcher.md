### Fixed

- Remote web3signer keymanager: `NewKeymanager` now waits for the key-file watcher to initialize before returning, and key-update notifications are sent without holding the keymanager lock, preventing a startup race and a potential lock-ordering deadlock.
