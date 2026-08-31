### Changed

- Web3Signer public keys are now tracked per source (flag, public-keys URL, key file) with the validating set as their union, so one source can no longer overwrite another's keys.
- `GET /eth/v1/remotekeys` marks keys owned by the key file as deletable instead of read-only.
- `POST /eth/v1/remotekeys` returns `error` instead of `imported` when `--validators-external-signer-key-file` is not set, since the keys would not be persisted.
- `DELETE /eth/v1/remotekeys` returns `error` naming the owning source for keys provided by the flag or the public-keys URL, and `not_found` for unknown keys.
- `--validators-external-signer-key-file` no longer has the flag and URL keys written into it at startup. Those keys still validate, they are just owned by their own source instead of being adopted by the file across a restart.
- `--validators-external-signer-poll-interval` is no longer ignored when a key file is configured. A poll now only replaces the URL's own keys, so it can run alongside the key file watcher.
