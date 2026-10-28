### Changed

- Stop reading a blocks-by-range response after the requested number of blocks, or `MAX_REQUEST_BLOCKS`, and return the blocks read so far.
