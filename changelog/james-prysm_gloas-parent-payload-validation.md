### Changed

- During initial sync, verify a Gloas block's parent execution payload envelope and its data columns before importing the block, and fetch the parent envelope by root when the range response does not include it.
