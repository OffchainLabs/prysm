### Fixed

- Protect `s.head.full` write in `postPayloadTasks` with `headLock` to avoid a data race with `setHead` and concurrent readers.
