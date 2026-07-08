### Fixed

- Emit the correct `parent_block_hash` in the Gloas `payload_attributes` SSE event by carrying the exact hash sent to the engine, so it matches for a full head instead of reporting the parent's hash.
