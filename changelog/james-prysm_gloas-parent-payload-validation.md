### Changed

- During initial sync, verify a Gloas block's required parent execution payload envelope and its data columns before importing the block, and fetch the parent envelope by root when it is missing from both the range response and local storage.
