### Fixed

- Validate sync committee messages submitted through `POST /eth/v1/beacon/pool/sync_committees` and the gRPC `SubmitSyncMessage` before broadcast. Prysm now checks the slot, the validator index, sync committee membership, and the signature, and returns 400 with per-index failures for invalid messages instead of gossiping them.
