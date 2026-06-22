### Fixed

- Validator client now surfaces a connection error when the beacon node rejects an event subscription with a non-200 status (e.g. HTTP 400 for an unsupported topic), instead of silently treating the error body as an empty event stream and dropping the subscription without any indication.
