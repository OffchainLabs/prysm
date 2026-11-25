### Fixed

- The gRPC `ProposeAttestationElectra` now verifies the attestation signature before saving it to the pool, so unsigned or forged votes are no longer packed into the node's own blocks. Discovered by Anthropic's OSS Scanner.
