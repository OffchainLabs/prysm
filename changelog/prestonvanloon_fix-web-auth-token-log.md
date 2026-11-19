### Fixed

- The validator web UI login URL, which contains the auth token, is now printed to the console instead of being written to the logs, so it no longer reaches the log files or the log stream endpoint. Discovered by Anthropic's OSS Scanner.
