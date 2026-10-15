### Fixed

- Send the genesis execution block hash as the safe hash before the first justification on a network with Gloas at genesis. Before, the safe hash was zero and besu rejected the forkchoice state.
