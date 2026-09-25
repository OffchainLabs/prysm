### Changed

- Cell-level dissemination for PeerDAS data columns (partial data columns) is now enabled by default. Use `--disable-partial-data-columns` to fall back to full column gossip.
- Deprecated `--partial-data-columns`; partial data columns are now the default, so the flag is a no-op.
