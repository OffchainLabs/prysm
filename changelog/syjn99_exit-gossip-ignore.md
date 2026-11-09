### Changed

- Ignore, instead of reject, `voluntary_exit` gossip when the validator has already initiated exit or has pending partial withdrawals.
- Check the `voluntary_exit` gossip exit epoch against the wall clock, with the `MAXIMUM_GOSSIP_CLOCK_DISPARITY` allowance, instead of the head state epoch.
