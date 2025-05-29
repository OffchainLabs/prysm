### Changed

- Add a context liveness check to some methods that iterate over objects or do expensive work. The routine can end early if the request is no longer alive.
