### Changed

- Builder API requests and responses now use SSZ encoding by default. Use `--disable-builder-ssz` to fall back to JSON.
- Deprecated `--enable-builder-ssz` (alias `--builder-ssz`); SSZ is now the default for builder setups, so the flag is a no-op.
