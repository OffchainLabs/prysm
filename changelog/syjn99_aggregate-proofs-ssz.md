### Added

- Accept SSZ (`application/octet-stream`) request bodies on `POST /eth/v2/validator/aggregate_and_proofs`. The body is decoded as the SSZ `List[SignedAggregateAndProof]`, fork-versioned by the `Eth-Consensus-Version` header.
