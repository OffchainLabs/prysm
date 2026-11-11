### Fixed

- Derive the node ID from the private key, so it is correct before the discovery listener starts and when discovery is disabled. Previously, at startup, the node briefly subscribed to the data column sidecar subnets of the all-zero node ID.
