### Changed

- Aggregate BLS public keys with one batched blst call on one core instead of one cgo call per key across GOMAXPROCS goroutines.
