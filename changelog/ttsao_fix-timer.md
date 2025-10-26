### Changed

- Replace `time.After()` with `time.NewTimer()` and explicitly stop the timer when the context is cancelled
