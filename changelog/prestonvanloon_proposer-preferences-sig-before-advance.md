### Fixed

- Verify the `proposer_preferences` gossip signature against the dependent state before advancing it to the lookahead epoch boundary, so a peer cannot force an epoch transition per message by sending preferences with invalid signatures.
