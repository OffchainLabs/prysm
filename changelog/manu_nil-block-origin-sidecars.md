### Fixed

- `fetchOriginSidecars`: guard against a `nil` block interface before calling `IsNil` to avoid a panic when the origin block is missing from the database.
