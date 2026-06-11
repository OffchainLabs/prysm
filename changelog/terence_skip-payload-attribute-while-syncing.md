### Fixed

- Skip payload attribute computation while the node is syncing, gated inside `getPayloadAttribute`. Computing attributes with a state far behind the wall clock processed thousands of slots per block, slowing sync to ~0.1 blocks/s.
