### Added 

- Configured the beacon node to seek peers when we have validator custody requirements. If one or more validators are connected to the beacon node, then the beacon node should seek a diverse set of peers such that broadcasting to all data column subnets for a block proposal is more efficient.
