### Added

- Support for checkpoint syncing from an unfinalized checkpoint, so a node can start from a recent anchor during a long period without finality. Supply the state and block with `--checkpoint-state` and `--checkpoint-block`; see `docs/unfinalized-checkpoint-sync.md`.
- `sync_origin_orphaned_suspected` metric, an actionable error log and a failing node status when peers finalize a chain that does not contain the checkpoint sync origin.

### Fixed

- Checkpoint sync no longer synthesizes the justified checkpoint epoch at the origin state's epoch. The origin node carries the state's real justified epoch, so the synthesized value made every block fail the fork choice viability check: head stayed pinned at the origin, no forkchoice update reached the execution client, and the node reported itself fully optimistic until a block justified a later epoch.
- `SaveOrigin` now verifies that the origin block matches the origin state's `latest_block_header` rather than trusting the pair.
- A checkpoint synced node no longer disconnects peers whose finalized checkpoint predates its origin, which it can neither prove nor disprove.
- The data column `earliest_available_slot` is no longer advertised below the checkpoint sync origin when the justified epoch trails the finalized epoch.
