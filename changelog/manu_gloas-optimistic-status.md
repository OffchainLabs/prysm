### Fixed

- Gloas: when a payload is validated, also validate the blocks already built on top of it. Before, these blocks stayed optimistic.
- Gloas: the head optimistic status is now refreshed when the head payload is received and when forkchoice nodes are validated.
