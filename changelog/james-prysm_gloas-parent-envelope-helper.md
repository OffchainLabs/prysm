### Changed

- Consolidate the Gloas envelope-matching helpers into `BlockBuiltOnParentEnvelope`. The initial-sync fetcher consistency check now also requires the envelope's beacon block root to equal the block's parent root.
