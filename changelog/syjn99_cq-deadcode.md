### Ignored

- Remove unused code left behind by earlier refactors in stategen, sync, backfill, das and the interval logger.
- Move test-only empty payload helpers out of the v1alpha1 proposer production code.
- Remove unused return values, always-nil errors, unused parameters and dead error checks.
- Enable the golangci `copyloopvar`, `unparam` and `unused` linters.
