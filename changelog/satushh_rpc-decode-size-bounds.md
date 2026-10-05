### Fixed

- Validate the RPC length prefix declared by a peer against the maximum SSZ size of the expected message type before allocating the receive buffer, instead of only the global `MAX_PAYLOAD_SIZE` cap. This prevents remote memory exhaustion through small-bodied RPC topics (such as `ping`) declaring multi-megabyte payloads and withholding the data.
