### Added

- `--engine-ssz-http` feature flag (off by default) scaffolding the REST + SSZ Engine API v2 transport ([execution-apis#793](https://github.com/ethereum/execution-apis/pull/793)). No behavior yet; JSON-RPC `engine_*` remains the default transport.
- SSZ-over-HTTP `GET /engine/v2/identity` support (`engine_getClientVersionV1` equivalent), behind `--engine-ssz-http`.
- SSZ-over-HTTP `POST /engine/v2/{fork}/payloads` (`engine_newPayload` equivalent) for the Amsterdam/Gloas envelope, behind `--engine-ssz-http`.
