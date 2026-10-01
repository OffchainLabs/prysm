### Fixed

- Make the validator client's startup retry loop observe a cancelled context immediately instead of waiting out the full reconnect backoff first.
