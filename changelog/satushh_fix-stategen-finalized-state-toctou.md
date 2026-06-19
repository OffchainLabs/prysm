### Fixed

- fixed a TOCTOU race in `stategen.latestAncestor` where a concurrent `SaveFinalizedState` between `isFinalizedRoot` and `FinalizedState` could return a finalized state belonging to a different block root than the one requested.
