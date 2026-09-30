### Changed

- Reduce state diff creation allocations with batched balance reads and direct encoding, without copying both balance lists or building an intermediate delta list.
