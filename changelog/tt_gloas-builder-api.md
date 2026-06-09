### Added

- Add builder-API client and types for the ePBS staked builder API: `getExecutionPayloadBid`, `submitSignedBeaconBlock`, `submitBuilderPreferences`, and the `DOMAIN_REQUEST_AUTH` signature domain.
- Pull execution payload bids from an external builder during Gloas block production, selecting the best of self-build, P2P, and Builder-API bids, and submit the signed block back to the builder when its bid wins.
- Submit per-builder proposer preferences (`max_execution_payment`) ahead of proposals via a new validator-client endpoint, authenticated with a signed request auth, and enforce the payment cap when validating builder bids.
- Query every builder a proposer signed a request auth for in parallel and select the best valid bid, so the validator-client `relays` list drives Gloas Builder-API multiplexing.
