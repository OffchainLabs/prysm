### Changed

- Implement `ValidatorBalances` in the validator client's beacon API chain client using the standard `/eth/v1/beacon/states/{state_id}/validators` endpoint instead of falling back to gRPC.
