### Fixed

- Skip the blocking data availability wait when importing a slot whose gossip window has closed, so a node whose head has fallen behind can catch up instead of stalling a full slot duration per block.
- Drain pending Gloas data columns before the pending payload envelope in the pending blocks queue, so the envelope's availability check sees the columns already gossiped for that root.
