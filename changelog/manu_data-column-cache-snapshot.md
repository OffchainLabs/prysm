### Changed

- Data column storage: write the filesystem cache to disk at graceful shutdown, and load it at the next startup instead of walking the whole data column storage.
