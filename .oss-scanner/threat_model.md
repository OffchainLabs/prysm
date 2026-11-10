# Prysm threat model

Prysm is a Go implementation of the Ethereum proof-of-stake consensus layer: a beacon node
(`cmd/beacon-chain`), a validator client (`cmd/validator`), and operator tooling (`cmd/prysmctl`,
`cmd/client-stats`). It runs a large share of mainnet validators. Correct, spec-conformant
consensus and availability are security properties: a bug in Prysm can split or stall the network,
get validators slashed, or expose signing keys. Scan the `develop` branch.

The Ethereum consensus specs (https://github.com/ethereum/consensus-specs, pinned per fork under
`specrefs/`) are the source of truth for protocol behaviour. `.agents/AGENTS.md` describes the
repository layout and conventions; read it and the scanned revision rather than assuming paths.

## Components and trust boundaries

### Beacon node (`beacon-chain/`)

- **`p2p/` and `sync/`**: libp2p gossipsub and req/resp. Every byte from a peer is untrusted: SSZ
  and Snappy decoding, request framing, gossip validation pipelines, rate limiting and peer scoring,
  initial sync and backfill, blob and data-column sidecar handling, discovery (discv5) packets.
- **`core/`** (per-fork state transition), **`blockchain/`** (block processing and fork choice,
  `forkchoice/`), **`state/`** (beacon state and caches), **`operations/`** (attestation, slashing,
  exit, BLS-change and deposit pools). Consensus invariants live here.
- **`db/`** (BoltDB, blob and column storage), **`cache/`**.
- **`rpc/`** and **`api/`**: the Beacon API (`rpc/eth/`), Prysm's own gRPC and REST API
  (`rpc/prysm/`), and node HTTP endpoints. These are unauthenticated and operators usually keep them
  local, but some expose them. Treat requests as untrusted and state which endpoint, flag and
  exposure a finding needs.
- **`execution/`**: Engine API client. The execution client is a single trusted endpoint
  authenticated with a JWT; a finding that needs a compromised execution client must say so.
- **`builder/`**: MEV-boost / builder API client. Relay responses (bids, headers, payloads) are
  semi-trusted and must be validated before use.
- Checkpoint sync (`--checkpoint-sync-url`), genesis state loading and `prysmctl` import paths
  consume operator-chosen but remote data. Check what is verified before it is trusted.

### Validator client (`validator/`)

- Holds or brokers signing keys: local keystores (`validator/keymanager/`), remote signing
  (`validator/keymanager/remote-web3signer`).
- Slashing protection (`validator/db/`, EIP-3076 interchange import and export) and duties and
  signing (`validator/client/`).
- The keymanager API (EIP-3030) and Prysm web API (`validator/rpc/`), authenticated by a bearer
  token. Any local or network client that can reach it is untrusted until the token checks pass.
- Signing safety and key confidentiality are the properties to protect. Producing a slashable
  signature is as damaging as key theft.

### Shared packages

`consensus-types/`, `proto/` (generated protobuf and SSZ), `encoding/ssz/`, `crypto/` (BLS via
`supranational/blst`, KZG), `config/params` (chain configs and presets), `network/`, `container/`,
`math/`, `runtime/`. Generated code is in scope when the generator or its inputs are wrong.

### Attackers to consider

- A remote peer, with any number of identities, sending arbitrary gossip, req/resp and discovery
  traffic.
- A malicious block producer, builder or relay sending well-formed but adversarial blocks, bids,
  payloads, blobs and data columns.
- Anyone who can reach an exposed Beacon API, gRPC port or validator web endpoint over the network.
- A checkpoint-sync or genesis source serving crafted state or blocks.
- A compromised execution client or remote signer is partially trusted: flag any assumption that
  requires attacker control of them.

Operators, their configuration files and flags, the local filesystem, and whoever holds the wallet
password are trusted. An attack that needs operator privileges or host access is not by itself a
vulnerability, but optional features stay in scope when the enabling flags are explicit.

## Security properties and areas of interest

- **Consensus correctness**: accepting an invalid block, attestation or sidecar; rejecting a valid
  one; fork choice diverging from the spec; wrong state roots; fork-transition handling;
  justification and finality; optimistic-sync status; a rejected object corrupting a shared cache
  or admitted chain state.
- **Validator safety**: anything that yields a slashable signature (double block, double or
  surround vote); a slashing-protection bypass, including through interchange import or database
  migration; signing with the wrong domain or fork; key material in logs, API responses or files;
  authentication bypass on the keymanager or web API.
- **Resource exhaustion from peers**: unbounded allocation on decode (SSZ lengths, Snappy
  expansion), expensive validation before cheap checks, queue and cache growth, goroutine leaks,
  and panics in message handlers. A single message that crashes or partitions a node is serious;
  check whether the handler recovers.
- **Memory safety**: cgo boundaries (blst, KZG), `unsafe` usage, and index bounds in SSZ codecs
  and generated code.
- **HTTP and API surfaces**: path traversal in database or backup endpoints, SSRF, host-header and
  CORS handling, token validation on the validator client.
- **Build and release**: `.github/workflows/`, `hack/` and release scripts, for credential
  exposure or injection. Lower priority than runtime code.

## How to rate severity

Use the Ethereum Foundation consensus-layer bug bounty tiers, applied to Prysm's share of the
network. Prysm runs a large fraction of mainnet validators, so a bug that affects every Prysm node
affects well over 5% of validators and often more than 33%.

- **Critical**: slash more than 50% of validators; finalize an invalid chain, or create ETH that
  the network finalizes; remote code execution on a beacon node or validator; signing-key theft at
  scale; taking down the entire network with a single message or on-chain transaction.
- **High**: slash more than 33% of validators, or every Prysm validator; trivially cause a network
  split affecting more than 33%; bring down more than 33% of the network (every Prysm node) with a
  single packet or on-chain transaction; remote key theft or slashing-protection bypass against a
  single validator.
- **Medium**: slash more than 1% of validators; trivially cause a split affecting more than 5%;
  bring down more than 5% of the network with a single packet or transaction; unauthenticated
  access to the keymanager or web API; a single-message remote crash of a beacon node that
  recovers automatically.
- **Low**: slashing with exotic prerequisites; high-effort single-peer denial of service
  (sustained, CPU or bandwidth intensive, or needing many packets); hardening gaps without a
  demonstrated exploit; issues that require operator privileges.

A performance regression alone does not establish exploitable denial of service: quantify attacker
cost against victim cost, amplification, reach and recovery. Distinguish a single-node crash from
demonstrated network-wide impact; do not claim slashing, theft or a network split from a local
crash alone. Publicly known issues are not findings.

## Building and testing in this image

The checkout is at `/src`. The binaries `beacon-chain`, `validator`, `prysmctl` and `client-stats`
are installed in `/usr/local/bin` and in `/src/dist`. The Go toolchain, module cache and a warm
build cache are kept; `GOPROXY=off`, `GOSUMDB=off` and `GOTOOLCHAIN=local` prevent downloads.
Bazel is the project's first-class build but is not installed here. Use `go` directly.

Spec-test fixtures were pre-fetched into `/src/third_party/testdata/` during the build. A fixture
missing from there cannot be fetched offline.

The maintainers' test passes, from `/src`:

```sh
go run ./build/test mainnet            # unit tests, -tags=develop, excluding e2e and spec tests
go run ./build/test mainnet-spectest   # consensus spec tests, mainnet preset
go run ./build/test minimal            # minimal-preset packages, -tags=develop,minimal
go run ./build/test minimal-spectest   # consensus spec tests, minimal preset
```

Targeted runs are much cheaper on the agent's 2 CPUs and 8 GB:

```sh
go test -tags=develop ./beacon-chain/sync/...
go test -tags=develop -run TestName ./beacon-chain/core/blocks/...
go test -tags=develop,minimal ./beacon-chain/rpc/prysm/v1alpha1/beacon/...
go test -tags=develop -run Finality ./testing/spectest/mainnet      # spec tests are flat packages
go test -tags=develop,minimal -run Sanity ./testing/spectest/minimal
```

`testing/endtoend` needs external binaries and a network and is not provisioned. Use the mocks and
helpers under `testing/` and in-process servers for reproducers; never contact public peers or live
networks.

Patches should be plain Go changes on the scanned revision. Do not worry about updating
`BUILD.bazel` files or the changelog; maintainers handle those.

## Out of scope or lower priority

- `testing/`, `tools/`, `hack/` and `build/`: test utilities, the e2e harness, spec-test drivers
  and developer scripts. Not shipped to node operators. Report only when a finding reaches shipped
  code.
- `third_party/` and third-party dependencies: report a dependency bug only when it is reachable
  through Prysm code, and explain the path.
- `cmd/client-stats` and `cmd/prysmctl`: operator-run tools, lower priority than the beacon node
  and validator client. Still in scope where they parse remote data.
- `contracts/`: generated deposit-contract bindings.
- Issues that need a compromised execution client, remote signer, operator or host on their own.

## Reporting

For each finding give the scanned commit, the component, the attacker's starting position, the
entry point, the validation already performed, the invariant violated and the concrete impact.
Include a deterministic reproducer, ideally a Go test with `-tags=develop`, the expected versus
actual behaviour, and a minimal patch with a regression test where practical. Group reports that
share a root cause. State uncertain reachability explicitly instead of dropping a candidate.

Send findings privately to `prysm@offchainlabs.com`. Follow `SECURITY.md` and
`.well-known/security.txt`; do not open public issues or publish unpatched details.
