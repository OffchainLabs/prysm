### Fixed

- Stop emitting a `chain_reorg` event, log and metrics when a post-Gloas head is re-saved for the same block root because its payload status flipped between empty and full.
