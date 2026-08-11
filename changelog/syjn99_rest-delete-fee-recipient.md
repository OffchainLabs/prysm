### Ignored

- Remove the unused `FeeRecipientByPubKey` method from the validator client. The proposer flow pushes fee recipients to the beacon node via `PrepareBeaconProposer` and never pulls them back, so the method had no callers. The beacon node's `GetFeeRecipientByPubKey` gRPC endpoint is unchanged.
