### Fixed

- Initial sync now requires every execution payload envelope in a batch to be matched and verified against a batch block before the block is marked full in forkchoice; a batch whose envelopes do not line up with its blocks is rejected.
