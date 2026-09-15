### Fixed

- Return `404` instead of `500` when a state ID cannot be resolved to a state root on `GET /eth/v1/beacon/states/{state_id}/root` and `POST /prysm/v1/beacon/states/{state_id}/query` (e.g. no block at the requested slot, slot in the future, missing genesis/finalized/justified block). An unparseable state ID on those endpoints now returns `400` instead of `500`.
- Return `404` instead of `500` on every state-fetching endpoint when the requested slot is in the future.
- Include the underlying reason in the `State not found` error message so callers can tell a skipped slot from a pruned one.
