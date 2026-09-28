### Fixed

- Initialize the finalized dependent root from the startup anchor block's parent root.
- gRPC `GetDuties` and `GetDutiesV2` read dependent roots from the state used for duties, including history before the checkpoint-sync anchor, without forkchoice lookups.
