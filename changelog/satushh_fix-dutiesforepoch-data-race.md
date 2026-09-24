### Fixed

- Fix data race on the shared `err` variable in `beacon-api` validator client's `dutiesForEpoch` where attester and proposer goroutines concurrently wrote to the same outer error.
