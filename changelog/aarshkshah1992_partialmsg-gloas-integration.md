### Changed

- Partial column broadcaster now decodes and processes incoming Gloas partial messages: it accepts the 41-byte Gloas group id and the header-less `PartialDataColumnSidecarGloas` wire. Because Gloas has no header to seed a verifier from, cells received before we have published the column locally are dropped rather than buffered.
- Proposer now only constructs partial data column sidecars for self-built blocks when partial column support (`--partial-data-columns`) is enabled, matching the execution client gating.
- Partial column broadcaster now skips republishing an already-published column when the publish adds no new cells, avoiding redundant per-peer publish passes in the EL reconstruction retry loop.
- Partial messages extension group state TTL is now derived from the broadcaster's group TTL (3 slots) instead of the 3-heartbeat gossipsub default, so per-peer state survives gaps between publishes.
