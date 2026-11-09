### Fixed

- Return an error from `slots.StartTime` when a slot's start time overflows `time.Duration`, instead of wrapping around to a time in the past. `slots.SinceSlotStart` now uses the same check.
