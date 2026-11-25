### Fixed

- Key the sync committee cache by the state's slot instead of the latest block header's slot, so entries for consecutive sync committee periods no longer overwrite each other when the header does not change across skipped slots. Discovered by Anthropic's OSS Scanner.
