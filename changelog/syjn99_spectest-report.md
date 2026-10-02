### Ignored

- Rework `hack/spectest-report.sh` to run the four CI spectest passes with `go test`, and compare against all pinned spec vectors, including cryptography-specs and fork choice compliance tests.
- Add missing spectests: Gloas `ptc_window`, custody groups and light client merkle proofs; Fulu light client update ranking; altair-deneb and Gloas `sync_committee_updates`; BLS `eth_aggregate_pubkeys`/`eth_fast_aggregate_verify`; KZG `blob_to_kzg_commitment`, `compute_blob_kzg_proof`, `verify_blob_kzg_proof`.
