### Fixed

- Validator client now restarts its beacon event stream after a fallback host switch or after the stream dies. Previously the stream stayed bound to the old beacon node until a full restart, silently stopping head and payload-availability events — and with them reorg-triggered proposer-preference resubmission — even though the rest of the client kept working against the new node.
- A replaced event stream is now stopped and drained before its successor starts, so two streams can never feed the validator client's event loop concurrently, and a stream shutdown can no longer close the shared events channel out from under its replacement.
