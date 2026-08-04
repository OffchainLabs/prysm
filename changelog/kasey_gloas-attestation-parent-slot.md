### Fixed

- Look up execution payload availability at the parent block's slot during Gloas attestation processing, per spec v1.7.0-alpha.13: `process_execution_payload_bid` now returns the parent slot, which is threaded through `process_operations` into `get_attestation_participation_flag_indices`.
- Only count blocks processed early in their own slot as proposer equivocation candidates in `should_apply_proposer_boost`, mirroring the spec's PTC timeliness condition.
