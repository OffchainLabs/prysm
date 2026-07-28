### Added

- `Received head event` log: Add `blockRoot` and `version`, and truncate the dependent roots. The `head_v2` variant now uses the same `Received head event` message, distinguished by `version=2`.
- `Listening to Beacon API events` log: Add the beacon node URL. (REST only.)
- `Starting event stream` log: Add the beacon node `host`, so the beacon node is identified for both the REST and the gRPC validator clients.
- `Submitted new attestations` and `Received head event` logs: Add `sinceSlotStartTime`.
- `Previous epoch voting summary` log: Add `slot`, the slot the validator attested to during the summarized epoch.

### Changed

- `Received head event` log: Omit `blockRoot` when the event carries no head block root, which is the case for the gRPC validator client since `StreamSlots` does not send it.
- `Previous epoch voting summary` log: Replace `startBalance`, `oldBalance`, `percentChange` and `percentChangeSinceStart` with `diffGwei`, and rename `newBalance` to `balanceEth`. `inactivityScore` is now only logged when non-zero.

### Removed

- Remove the `Request count did not match included validator count. Only keys that have been activated will be included in the request.` log, printed once per slot as soon as some validators are not active, which is a perfectly normal situation.
