### Fixed

- `bytesutil.FromBytes4` no longer zeroes bytes 4 through 7 of its input slice when the caller passes a slice longer than 4 bytes.
