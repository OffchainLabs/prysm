### Added

- Gloas: reject `execution_payload_bid` gossip when the builder version is not `PAYLOAD_BUILDER_VERSION`, the blob KZG commitment count exceeds the per-slot limit, or `prev_randao` does not match the RANDAO mix.
