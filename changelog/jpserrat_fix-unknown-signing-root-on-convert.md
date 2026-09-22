### Fixed

- Do not treat a missing signing root as the all-zero signing root when converting a minimal slashing protection database into a complete one, or when exporting proposal history. [#17516](https://github.com/OffchainLabs/prysm/issues/17516)
- Do not blacklist a public key when an EIP-3076 interchange file lists the same attestation twice without a signing root. [#17516](https://github.com/OffchainLabs/prysm/issues/17516)
