### Fixed

- Gossip blocks building on the empty parent are no longer rejected when the parent's revealed payload was invalid, only blocks whose bid builds on the invalid payload are rejected.
