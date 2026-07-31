### Fixed

- Validator client (post-Gloas split duties only): refetch the current-epoch duties each slot when the epoch-boundary fetch fails, so a transient beacon-node failure no longer leaves the validator without duties for the rest of the epoch.
