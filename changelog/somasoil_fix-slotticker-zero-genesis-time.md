### Fixed

- Use `time.Time.IsZero()` instead of `Unix() == 0` when validating the genesis time in `NewSlotTickerWithOffset` and `NewSlotTickerWithIntervals`, so a zero `time.Time` is correctly rejected and a legitimate Unix epoch genesis time is not.
