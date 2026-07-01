### Fixed

- Re-push proposer preferences when the validator client connects to a different beacon node. The submitted-slot dedup cache survives runner restarts and beacon-node fallback switches, so a freshly connected node previously received no preferences for slots already marked submitted. A new runner (initial connect / health recovery) now propagates `forceFullPush` to the proposer-preference build, and a beacon-node connection change forces a full re-push. The change is detected via a monotonic connection counter (`NodeConnection.ConnectionGeneration`) rather than the host string, so a round-robin bounce (host0 → host1 → host0) that replaces the connection is still caught.

### Added

- Add `ConnectionCounter` to the REST connection provider and `ConnectionGeneration` to `NodeConnection`, exposing a monotonic counter that advances on each beacon-node fallback switch (the gRPC provider already had `ConnectionCounter`).
