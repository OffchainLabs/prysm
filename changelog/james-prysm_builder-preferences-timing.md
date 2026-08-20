### Changed

- Validator client now submits Gloas builder preferences once per upcoming proposal slot, on the same schedule as proposer preferences, instead of re-sending them every slot; restarts and beacon host switches still re-push everything.
- Reorg-driven duty changes now re-push builder preferences alongside proposer preferences.
