### Fixed

- Fix concurrent map iteration and write fatal in the data-column/blob data-availability wait by logging the slot-end warning from the wait loop instead of a timer goroutine.
