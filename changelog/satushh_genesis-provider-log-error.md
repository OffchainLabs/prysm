### Fixed

- Include the underlying error in the `genesis provider failed` warning log, so failures such as a misconfigured checkpoint sync URL are visible instead of surfacing later as a generic `genesis state has not been initialized` error.
