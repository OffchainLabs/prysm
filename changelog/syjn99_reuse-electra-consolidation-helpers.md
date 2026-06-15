### Ignored

- Reuse `electra` consolidation helpers from `requests` instead of duplicating them, and relocate the `gloas`-only `MatchingPayload`/`InitiateBuilderExit`/`RemoveBuilderPendingPayment` helpers to their sole callers (`altair`/`blocks`) to resolve the `requests`→`electra` import cycle.
