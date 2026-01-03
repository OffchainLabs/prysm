### Changed

- the /eth/v2/beacon/pool/attestations and /eth/v1/beacon/pool/sync_committees now returns a 503 error if the node is still syncing, the rest api is also working in a similar process to gRPC broadcasting immediately now.