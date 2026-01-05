### Changed

- Replaced `time.Sleep` with `require.Eventually` polling in tests to fix flaky behavior caused by race conditions
  between goroutines and assertions.
