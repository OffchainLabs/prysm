### Added

- Implement EIP-8148 (custom sweep threshold for validators) as part of the Gloas fork rather than Heze.
- DEVNET ONLY: mock EIP-8148 set sweep threshold requests via `POST /prysm/v1/debug/beacon/sweep_threshold_requests`, injected into locally built payloads and never sent to the execution client.
