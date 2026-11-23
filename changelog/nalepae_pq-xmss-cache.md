### Added

- PQ fork: at start, the validator client reads the XMSS trees of its keys from `<datadir>/xmss-cache`. If they are missing or invalid there, it fetches them from the published tree cache at `--xmss-cache-url`. If neither has them, it builds them from the seed. It then writes the trees to `<datadir>/xmss-cache` for the next start.
