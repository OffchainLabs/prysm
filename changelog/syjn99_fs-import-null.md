### Fixed

- Minimal slashing protection database: a `null` entry in `signed_blocks` of an imported EIP-3076 file no longer stops the import of later blocks for that key, and a `null` entry in `signed_attestations` no longer panics.
