### Fixed

- Fixed regression in find peer functions introduced in PR#15471, where nodes with equal sequence numbers were incorrectly skipped and the peer count was incorrectly reduced when replacing nodes with higher sequence numbers.