### Fixed

- Retry blob sidecar reconstruction from the execution layer on transient failures, using singleflight to deduplicate concurrent requests.
