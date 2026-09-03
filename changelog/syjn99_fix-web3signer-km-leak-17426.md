### Fixed

- Initialize the keymanager once per validator runner instead of on every beacon-node connection retry, which leaked a key watcher goroutine (web3signer URL poller or wallet/file watcher) and a duplicate accounts-changed subscription per retry. (fixes #17426)
