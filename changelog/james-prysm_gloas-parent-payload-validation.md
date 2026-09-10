### Changed

- During initial sync, verify a Gloas block's required parent execution payload envelope and its data columns before importing the block, and recover missing parent envelopes by root with a historical range fallback when they are not stored locally.
