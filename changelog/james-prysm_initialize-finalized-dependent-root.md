### Fixed

- Initialize the finalized dependent root from the startup anchor block's parent root.
- gRPC `GetDuties` and `GetDutiesV2` return a zero previous dependent root instead of an error when forkchoice cannot resolve it below its tree root.
