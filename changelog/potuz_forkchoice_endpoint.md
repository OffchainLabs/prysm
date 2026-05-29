### Added
- New Gloas-aware fork-choice dump endpoint `GET /eth/v2/debug/fork_choice` that emits one entry per `(root, payload_status)` tuple (PENDING / EMPTY / FULL) and exposes PTC attester counts on the PENDING entry.

### Changed
- Forkchoice now tracks distinct PTC attesters via a shared voted-mask bitfield; repeat votes from the same committee index overwrite the previous vote.
