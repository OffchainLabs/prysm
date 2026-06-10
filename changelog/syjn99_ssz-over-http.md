### Added

- `--engine-ssz-http` feature flag (off by default) scaffolding the REST + SSZ Engine API v2 transport ([execution-apis#793](https://github.com/ethereum/execution-apis/pull/793)). No behavior yet; JSON-RPC `engine_*` remains the default transport.
- SSZ-over-HTTP `GET /engine/v2/identity` support (`engine_getClientVersionV1` equivalent), behind `--engine-ssz-http`.
- SSZ-over-HTTP `POST /engine/v2/{fork}/payloads` (`engine_newPayload` equivalent) for the Amsterdam/Gloas envelope, behind `--engine-ssz-http`.
- SSZ-over-HTTP `POST /engine/v2/{fork}/forkchoice` (`engine_forkchoiceUpdated` equivalent) for the Osaka/Fulu and Amsterdam/Gloas forks, behind `--engine-ssz-http`. The opaque `payload_id` is echoed verbatim; problem+json errors map onto the existing forkchoice/attribute sentinels.
- SSZ-over-HTTP `GET /engine/v2/{fork}/payloads/{id}` (`engine_getPayload` equivalent) for the Osaka/Fulu and Amsterdam/Gloas forks, behind `--engine-ssz-http`. The opaque id is hex-encoded into the path and the response is never cached.
