### Fixed

- Ignore a gossiped sync committee contribution only when a seen contribution already has all of its aggregation bits, per the Altair p2p spec. Before, Prysm ignored any contribution whose bits overlapped a seen one.
