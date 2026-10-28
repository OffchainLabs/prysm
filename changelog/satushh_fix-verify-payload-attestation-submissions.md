### Fixed

- Verify payload attestation message signatures before broadcasting in the `POST /eth/v1/beacon/pool/payload_attestations` handler and the gRPC `SubmitPayloadAttestation` endpoint, instead of only checking that the signature bytes parse.
