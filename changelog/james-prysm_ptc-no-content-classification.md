### Fixed

- Report the beacon node's own error instead of `context deadline exceeded` when a retrying REST request runs out of time. Callers can now tell an expected response such as `204 No Content` from a real failure, rather than logging both as errors.
