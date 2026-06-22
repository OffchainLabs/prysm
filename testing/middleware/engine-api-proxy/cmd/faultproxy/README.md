# faultproxy

Engine-API proxy with a toggleable `SYNCING` fault, for testing optimistic sync.
It sits between a CL and its EL, forwards engine JSON-RPC, and on demand makes
`newPayload` / `forkchoiceUpdated` return `SYNCING` so the CL goes optimistic.

Mirrors the fault the Go E2E injects via `EngineProxy.AddRequestInterceptor`,
but driven over HTTP instead of in-process.

## Toggle

```bash
curl -fsS -X POST   http://<host>:8552/fault/syncing   # enable
curl -fsS -X DELETE http://<host>:8552/fault/syncing   # disable (replays held payloads)
```

## Run standalone

Flags mirror `json_rpc_snoop`: `-b` host, `-p` listen port, positional EL engine URL.
With no `--jwt-secret-file` it forwards the caller's `Authorization` header (passthrough).

```bash
go run . -b=0.0.0.0 -p=8551 http://localhost:8551 --admin-addr=:8552
```

## Use in Kurtosis (ethereum-package snooper drop-in)

Build (context = repo root):

```bash
docker build -t prysm-faultproxy:local \
  -f testing/middleware/engine-api-proxy/cmd/faultproxy/Dockerfile .
```

Enable the snooper on the participant and point it at the image:

```yaml
participants:
  - cl_type: prysm
    el_type: geth
    snooper_enabled: true
snooper_params:
  image: prysm-faultproxy:local
```

Prysm's `--execution-endpoint` now routes through the proxy. Toggle from another
enclave service (e.g. assertoor) via the engine snooper's DNS name:
`http://snooper-engine-<idx>-<cl>-<el>:8552/fault/syncing`
(first participant → `snooper-engine-1-prysm-geth`).

Note: don't pass `--admin-addr` through snooper `extra_args` — Go's flag parser
stops at the positional EL URL the snooper appends first. The `:8552` default is
what you want anyway.
