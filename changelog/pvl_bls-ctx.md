### Fixed

- Broadcasting BLS to execution changes should not use the request context in a go routine. Use context.Background() for the broadcasting go routine.
