### Fixed

- Stop `SlotTicker` and `SlotIntervalTicker` goroutines from leaking when nothing reads `C()` anymore. `Done()` now closes its channel instead of sending on it from a spawned goroutine, so it can interrupt a ticker that is already blocked delivering a tick, and calling it more than once no longer leaks a goroutine per call.
