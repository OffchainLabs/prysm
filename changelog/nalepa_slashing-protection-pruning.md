### Changed

- Validator client: refuse to sign a block or attestation data that the beacon node returned for another slot than the duty slot.
- Validator client: when the complete slashing protection database prunes old proposals or attestations, raise the EIP-3076 lowest signed slot / source / target watermarks to the pruning cutoff.
