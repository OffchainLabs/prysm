### Security
- Verify gossip payload envelope signatures without loading the block's state: the builder pubkey is recorded in forkchoice at block insertion and the self-build proposer pubkey is read from the head state.
