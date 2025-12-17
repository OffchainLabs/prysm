### Changed

- the submit attestation v2 endpoint now returns a 503 error if the node is still syncing, the rest api is also working in a similar process to gRPC broadcasting immediately now.