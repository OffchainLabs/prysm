### Fixed

- `SubmitAttestationsV2` and the gRPC `ProposeAttestation` now apply the gossip time-window and target-epoch checks to the attestation slot before computing committees, and `ValidateAttestationTime` no longer accepts slots whose start time overflows. Discovered by Anthropic's OSS Scanner.
