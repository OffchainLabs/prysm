### Fixed

- Use the payload's own parent hash when pruning fork choice after the execution engine rejects a block's payload, as batch import already does, so valid optimistic ancestors are no longer removed.
