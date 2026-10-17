### Added

- Implement EIP-8365 (disallow new 0x00 validators): from the Heze fork, pending deposits that would create a new validator with BLS (`0x00`) withdrawal credentials are skipped. Top-ups to existing validators, including validators created earlier in the same epoch, are unaffected. The rule is gated on `HEZE_FORK_EPOCH` and is inactive on all networks.
