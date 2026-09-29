### Fixed

- Release the read lock on the no-keys early return in the remote web3signer keymanager's `DeletePublicKeys`, which previously deadlocked all subsequent key updates.
