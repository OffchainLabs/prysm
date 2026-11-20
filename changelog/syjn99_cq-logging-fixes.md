### Fixed

- Mask credentials in the builder URL logged at start-up, in the checkpoint sync URL logged for `--genesis-beacon-api-url`, and in the proposer settings URL.
- Log the failing trusted peer, not the whole trusted peer list, when its address cannot be resolved.

### Changed

- Log validator registrations for pubkeys not yet in the validator set at debug level, not one warning per key per epoch.
- Log blobs still missing at slot end as a warning, the same as the data column path.
- Log broadcast peer searches that time out at debug level, not as errors.
- Do not log errors that are also returned: the checkpoint sync origin block commitment error and light client RPC rate-limit failures.
