### Fixed

- Pack Electra attestations by marginal proposer reward. The proposer used to score each candidate on-chain aggregate against the pre-block state in isolation and then take the best `MAX_ATTESTATIONS_ELECTRA`, which filled blocks with aggregates whose votes were already counted by an earlier attestation in the same block, or by an already-imported block. Candidates are now picked by what they add on top of the ones already picked, and packing stops once nothing left adds a vote.
