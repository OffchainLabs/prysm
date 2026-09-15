### Added
- `--enable-experimental-batch-submission`: batches per-slot attestations into one `POST /eth/v2/beacon/pool/attestations` call instead of per-validator requests in REST validator client.

### Changed
- REST validator client submits SSZ-encoded attestations first with JSON fallback for `POST /eth/v2/beacon/pool/attestations`.