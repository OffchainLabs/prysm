### Fixed

- Validator client (post-Gloas split duties only): fixed a regression where fetching next-epoch duties at the epoch boundary blocked duty performance (attestations/proposals); next-epoch duties are now fetched in the background.
