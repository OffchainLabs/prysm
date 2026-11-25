### Fixed

- Reject invalid filesystem slashing-protection records and persist updates using synchronized atomic replacement to preserve validator protection history.
- Preserve Windows filesystem slashing-protection writes with write-through replacement and add context to persistence errors.
