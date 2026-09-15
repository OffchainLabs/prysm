### Changed

- Store epoch-precompute validators and Altair attestation deltas as value slices (`[]precompute.Validator`, `[]altair.AttDelta`) instead of slices of per-validator heap-allocated pointers, eliminating roughly two heap objects per validator per epoch transition.
