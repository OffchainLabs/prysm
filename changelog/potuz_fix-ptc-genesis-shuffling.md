### Fixed

- PTC attestations are now produced during the first two epochs. The shuffling check compared a dependent root that falls back to the origin block root against one that reports the zero root until the first finalization, so it never matched before epoch 2.
