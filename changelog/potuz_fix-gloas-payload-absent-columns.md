### Fixed

- Initial sync no longer requests data column sidecars for payload-absent Gloas slots, which previously wedged sync on `respondedSidecars=0`. Columns are now requested by revealed payload envelopes instead of the bid's `blob_kzg_commitments`.
