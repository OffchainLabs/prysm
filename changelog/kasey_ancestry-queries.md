### Changed
- More efficient query method for stategen to retrieve blocks between a given state and the replay target block. This avoids attempting to look up blocks that are not needed for head replay queries, which may be missing due to a previous rollback bug.
