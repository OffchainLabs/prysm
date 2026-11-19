### Fixed

- Fix a goroutine leak in the `payload_attributes` SSE event when the stream drops the reader or its context ends before the result is read.
