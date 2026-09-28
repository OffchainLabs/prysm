### Changed

- Proposer settings loaded from `--proposer-settings-file` or `--proposer-settings-url` now reject unknown keys, the internal `builders_set` marker, and `version` values above 2 instead of silently ignoring them. A misspelled key such as `fee_recipent` used to fall back to the default fee recipient without any log; it is now a startup error. The legacy v1 `relays` key is still accepted and ignored.
