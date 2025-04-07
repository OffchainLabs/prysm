### Fixed

- [Sync] Fixed blob sidecar validation to ensure the number of available blob sidecars matches the number of KZG commitments in the block before responding to BlobSidecarsByRange requests. This prevents Prysm nodes from serving an incorrect number of blob sidecars.
