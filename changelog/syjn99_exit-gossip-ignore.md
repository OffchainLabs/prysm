### Changed

- Ignore, instead of reject, `voluntary_exit` gossip when the exit epoch is in the future, the validator has already initiated exit, or the validator has pending partial withdrawals, per consensus-specs#5596 and #5764.
