### Added

- Added the `PartialDataColumnGroupID` and `PartialDataColumnSidecarGloas` SSZ containers and a Gloas partial data column constructor. `PartialDataColumn` now embeds the fork-abstracted `RODataColumn`, so Gloas partial columns source their KZG commitments from the block's execution payload bid and never eager-push a header (Gloas has none).
- Construct and gossip partial data columns for Gloas blocks at every site Fulu does: the execution payload envelope (self-build and builder REST), EL reconstruction, the gossip-receipt republish, and the pending-column queue. Gated behind the new opt-in `--enable-gloas-partial-columns` flag.

### Changed

- Partial data column hardening: ignore empty-commitment partial-column headers up front (without downscoring the peer) rather than relying on later verification, and skip building inert, cell-less partial columns for blocks with no blob commitments.
