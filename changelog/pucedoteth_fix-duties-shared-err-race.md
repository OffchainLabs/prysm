### Fixed

- Fix a data race in the REST validator client: the attester and proposer duty fetches for an epoch ran concurrently but assigned the same `err` variable, so one goroutine could observe the other's error. This could report a proposer failure as an attester failure, or drop an attester error and then panic on the nil response.
