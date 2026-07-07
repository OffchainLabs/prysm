### Fixed

- Re-push proposer preferences when the validator client connects to a different beacon node. The submitted-slot dedup cache survives runner restarts and beacon-node fallback switches, so a freshly connected node previously received no preferences for slots already marked submitted. A new runner (initial connect / health recovery) now propagates `forceFullPush` to the proposer-preference build, and a beacon-node connection change forces a full re-push. The change is detected via a monotonic connection counter (`ValidatorClient.ConnectionGeneration`, implemented by each transport client from its own connection provider) rather than the host string, so a round-robin bounce (host0 → host1 → host0) that replaces the connection is still caught. The switch signal is only consumed once a push is confirmed, and proposer preferences and builder registrations are tracked independently, so a failed re-push to the new node is retried on later slots.
- Retry failed proposer-preference submissions: a batch whose submission fails releases its dedup-cache reservations so the per-slot rebuild resubmits it, covering both the regular push and the reorg resubmission path.
- Detach the proposer-preference submission from the slot context so it can no longer be cancelled before the mid-slot submit delay elapses (matching the builder-preference submission).

### Added

- Add `ConnectionCounter` to the REST connection provider and `ConnectionGeneration` to the validator client interface, exposing a monotonic counter that advances on each beacon-node fallback switch (the gRPC provider already had `ConnectionCounter`).
