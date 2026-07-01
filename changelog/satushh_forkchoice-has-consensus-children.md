### Changed

- Avoid cloning a slice just to count it in the Gloas fork-choice best-descendant walk: `updateBestDescendantConsensusNode` and `tips` now use a `hasConsensusChildren` helper instead of `len(allConsensusChildren(...))`, removing a per-node heap allocation.
