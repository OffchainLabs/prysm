### Fixed

- A JSON `null` element in a beacon REST list submission (attestations, aggregates, proposer preferences, payload attestations) no longer panics the handler and drops the connection; it is reported as a per-index failure.
- `POST /eth/v2/beacon/pool/attestations` now reports per-index failures against the submitted list instead of a compacted one, so a request mixing bad and good attestations no longer labels every failure `index: 0`.

### Changed

- An empty SSZ request body on the proposer preferences and payload attestation endpoints now returns `No data submitted` instead of an SSZ list-size error.
- Per-index decode failure messages are no longer prefixed per endpoint: SSZ failures read `could not decode SSZ message: …`, JSON failures carry the converter error alone.

### Ignored

- Five duplicated JSON/SSZ indexed-list decoder pairs in the beacon REST layer, replaced by `server.ConvertList`, `server.DecodeJSONList` and `server.DecodeSSZList`.
