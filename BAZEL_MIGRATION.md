# Bazel → Go-toolchain Migration Plan

Goal: remove Bazel entirely from Prysm, replacing each task with the Go toolchain
where possible, and Docker / small purpose-built tools where not.

## Key finding that shapes everything

**All generated code is already committed to the source tree** (36 `*.pb.go`, 13
`*.ssz.go`), and `go build ./...` / `go test ./...` already succeed without Bazel.

Bazel's `go_library` / `go_binary` / `go_test` graph is therefore **not** load-bearing
for day-to-day compile/test. What Bazel actually provides that we must replace is the
*peripheral* tooling:

1. Code generation (proto, SSZ, mocks) — regenerating the committed files
2. The `minimal` vs `mainnet` SSZ variant selection
3. CGO cross-compilation (blst, libp2p, hashtree) via a hermetic C toolchain
4. Docker/OCI image building & pushing
5. `.deb` packaging
6. `nogo` static analysis (staticcheck + ~30 custom analyzers)
7. Version stamping
8. CI orchestration (spec tests, e2e, gazelle checks)

This means the migration is mostly **replacing satellites, not the core build** — much
lower risk than it first appears.

---

## How the tricky mechanisms work today (so we can replicate them)

### SSZ codegen + network variants
- `.proto` files contain template placeholders (e.g. `{block_roots.size}`).
- `proto/ssz_proto_library.bzl` holds two dicts, `mainnet` and `minimal`, and does
  text substitution on the `.proto` before compilation (`ssz_proto_files` rule).
- After protoc produces `*.pb.go`, the `ssz_gen_marshal` rule (`tools/ssz.bzl`) runs
  `fastssz/sszgen` over the package to emit `*.ssz.go`.
- `bazel build --config=minimal` sets **both** `--//proto:network=minimal` (minimal
  substitution dict) **and** the Go build tag `minimal` (`.bazelrc:30-31`).
- Hand-written variant files already use Go build tags today:
  `sync_committee_{mainnet,minimal}.go`, `payload_attestation_{mainnet,minimal}.go`,
  `committee_bits_minimal.go`, `config/fieldparams/minimal.go`, etc.
- **Only the mainnet `*.ssz.go` is committed.** Minimal is regenerated on the fly by
  Bazel. This is the one real design gap for a pure-Go flow (see Phase 3).

### Proto plugins
- Custom compiler `protoc-gen-go-cast` (+ `_grpc` variant) — supports the `(ext.cast_type)`
  options used throughout the protos. It's a normal Go protoc plugin (installable via
  `go install`).

### nogo
- One Bazel-internal binary running staticcheck + golang.org/x/tools passes + ~30 custom
  analyzers in `tools/analyzers/`. Config/exclusions in `nogo_config.json`.

---

## Migration phases (ordered, each independently shippable)

### Phase 0 — Baseline & scaffolding
- Add a `Makefile` (or `magefile`) as the new task entrypoint. Targets to fill in across
  phases: `build`, `test`, `gen`, `gen-proto`, `gen-ssz`, `gen-mocks`, `lint`, `docker`,
  `deb`, `cross`.
- Record the current Bazel-produced outputs as golden references:
  - `bazel build //... ` binary hashes per platform
  - `git stash` of regenerated `*.pb.go` / `*.ssz.go` to diff against new codegen
- No Bazel removed yet. Everything additive.

### Phase 1 — Code generation without Bazel
Build a single orchestrator (recommended: a small Go program `tools/cmd/gen` invoked by
`go generate`, so it is itself buildable with the Go toolchain). It performs:

1. **Template substitution**: port the `mainnet`/`minimal` dicts from
   `proto/ssz_proto_library.bzl` into Go (or a JSON file) and expand the `{...}`
   placeholders in `.proto` into a temp dir.
2. **Proto compile**: run `protoc` (or `buf`) with the local plugins:
   - `go install github.com/prysmaticlabs/protoc-gen-go-cast@<pin>`
   - `protoc --go-cast_out=... --go-cast-grpc_out=...` (+ google/api annotations include
     path, currently resolved via gazelle).
   - Replaces `hack/update-go-pbs.sh`.
3. **SSZ compile**: run `fastssz/sszgen` per package over the generated `*.pb.go`,
   honoring the per-package `objs` / `exclude_objs` lists currently encoded in each
   `BUILD.bazel`. Port those lists into the generator config. Strip the `// Hash:` line
   (as `hack/update-go-ssz.sh` does today). Replaces `hack/update-go-ssz.sh`.
4. **Mocks**: keep `hack/update-mockgen.sh` (already pure `mockgen`); optionally convert
   to `//go:generate` directives.

Tooling choice: **`protoc` + plugins driven by a Go orchestrator** is recommended over
`buf` because the pre-compile template substitution step doesn't fit `buf`'s in-place
model cleanly. `buf` remains a viable alternative if we restructure templating.

Acceptance: regenerated files are byte-identical (modulo the stripped Hash line) to the
committed Bazel-generated ones.

### Phase 2 — Build & version stamping
- `go build` for all binaries. Replace `--stamp` / `workspace_status.sh` with
  `-ldflags "-X .../runtime/version.gitCommit=$(git rev-parse HEAD) -X ...=$TAG -X ...=$DATE"`.
  Check `runtime/version` for the existing stamped vars (currently fed via a Bazel
  `version_file` genrule) and point ldflags at them.
- Makefile `build` target per binary (beacon-chain, validator, prysmctl, + tools).
- PGO: pass `-pgo=pprof.beacon-chain.samples.cpu.pb.gz` to `go build` (Go has native PGO,
  replacing the Bazel `pgoprofile` attribute).

### Phase 3 — minimal vs mainnet as a pure Go build tag
This is the one genuine design change. Today minimal SSZ is regenerated on demand; Go
can't do that mid-build. Options:

- **(Recommended) Commit both variants behind build tags.** Generate
  `*.ssz.mainnet.go` (`//go:build !minimal`) and `*.ssz.minimal.go` (`//go:build minimal`)
  in Phase 1's orchestrator, commit both. Then `go build` = mainnet, `go build -tags=minimal`
  = minimal — matching the existing hand-written tagged files. Cost: ~13 extra committed
  files; codegen must run twice (once per dict).
- Alternative: regenerate before minimal test runs (a `make test-minimal` that runs
  codegen with the minimal dict first). Simpler to set up, but stateful/dirty-tree — not
  recommended.

### Phase 4 — CGO cross-compilation
Prysm needs CGO (blst, hashtree, some libp2p). Replace the hermetic Zig toolchain
(`hermetic_cc_toolchain`, already in the Bazel setup) with **`zig cc` as the C compiler**:
```
GOOS=linux  GOARCH=amd64 CGO_ENABLED=1 CC="zig cc -target x86_64-linux-gnu"   go build ...
GOOS=linux  GOARCH=arm64 CGO_ENABLED=1 CC="zig cc -target aarch64-linux-gnu"  go build ...
GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 CC="zig cc -target aarch64-macos"      go build ...
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC="zig cc -target x86_64-windows-gnu" go build ...
```
- Port the ARM64 optimization flags from `build/bazelrc/cross.bazelrc`
  (`-march=armv8-a -ftree-vectorize ...`) into `CGO_CFLAGS`.
- Pin Zig version (currently 3.0.1 via `hermetic_cc_toolchain`). `xx` / `goreleaser` can
  manage this, or a thin wrapper script in `tools/cross-toolchain/`.
- Validate blst SIMD variants (`blst_modern` / portable builds) map to the right CGO
  flags (Bazel uses `--define=blst_modern=false` for the portable image).

### Phase 5 — Docker / OCI images
Replace `rules_oci` + `prysm_image.bzl` with **multi-stage Dockerfiles + `docker buildx`**:
- Cross-compile binaries in Phase 4, `COPY` into a thin `gcr.io/distroless/cc` (or the
  existing Debian 11 base) runtime stage.
- `docker buildx build --platform linux/amd64,linux/arm64 --push -t gcr.io/offchainlabs/prysm/<bin>:<tag>`
  gives the multi-arch manifest that `oci_image_index` produced.
- One Dockerfile per binary (or one parameterized). Preserve labels
  (`org.opencontainers.image.source`), entrypoints, and the backwards-compat symlinks.
- Replaces `hack/build_and_upload_docker.sh`.

### Phase 6 — `.deb` packaging
Replace `rules_pkg` (`pkg_deb`/`pkg_tar`) with **`nfpm`** (single YAML → deb/rpm/tar):
- One `nfpm.yaml` each for `prysm-beacon-chain` and `prysm-validator`, bundling binary +
  config + systemd unit (mirror `beacon-chain/package/` and `validator/package/`).

### Phase 7 — Static analysis (nogo → standalone vettool)
Replace `nogo` with a **single `multichecker` binary** (`tools/cmd/prysm-vet`) built from
`golang.org/x/tools/go/analysis/multichecker`, embedding:
- staticcheck analyzers (SA*, honoring the sa1019 exclusion)
- the golang.org/x/tools passes currently enabled
- all custom analyzers in `tools/analyzers/` (already standard `analysis.Analyzer`s)

Run as `go vet -vettool=$(go run ./tools/cmd/prysm-vet) ./...`, or call multichecker
directly. Port `nogo_config.json` exclusions into per-analyzer flags or a config the
runner reads. `golangci-lint` is the alternative, but a hand-rolled multichecker
preserves the exact analyzer set with least behavioral drift. Keep existing
`golangci-lint`/`gosec` CI steps as-is.

### Phase 8 — Tests, spec tests, e2e
- Unit tests: `go test ./...` (`-race`, `-cover` native). Minimal: `go test -tags=minimal ./...`.
- Spec tests: replace the `download_spectests.bzl` repo rule with a script that downloads
  the consensus-spec-tests tarball into a cache dir, then `go test ./testing/spectest/...`
  with the spec dir via env var (already `SPEC_TEST_REPORT_OUTPUT_DIR`).
- E2E: the `testing/endtoend` Bazel transitions (minimal/mainnet) become `go test` with
  the `minimal` tag + binaries built in Phase 2; binary paths injected via env/flags
  instead of Bazel `data` deps.

### Phase 9 — CI/CD rewiring
- Rewrite `.github/workflows/*.yml` and Buildkite to call the Makefile targets
  (`make gen && git diff --exit-code` replaces `check_gazelle.sh` + the proto/ssz
  generated-code check).
- Drop `gazelle` / `deps.bzl` entirely — `go.mod` is the single source of truth.
- Docker push job → `docker buildx` (Phase 5).

### Phase 10 — Delete Bazel
Once Phases 1–9 are green in CI, remove:
- `WORKSPACE`, `MODULE.bazel`, `.bazelrc`, `.bazelversion`, `.buildkite-bazelrc`,
  `bazel.sh`, `deps.bzl`, `distroless_deps.bzl`
- all `BUILD.bazel` files (`git ls-files '*BUILD.bazel' | xargs rm`)
- `tools/ssz.bzl`, `proto/ssz_proto_library.bzl`, `tools/prysm_image.bzl`,
  `tools/cross-toolchain/`, `tools/image_deps.bzl`, `tools/download_spectests.bzl`,
  `build/bazelrc/`, `tools/nogo_config/`, `nogo_config.json`
- the `bazelbuild/rules_go` line in `go.mod` if unused after migration
- gazelle directives are mooted by deleting the BUILD files.

---

## Risk register
| Area | Risk | Mitigation |
|---|---|---|
| Proto codegen | byte drift vs Bazel output (plugin/protoc versions) | pin protoc + plugin versions; diff against committed files in Phase 0 |
| SSZ minimal | no committed minimal variant today | Phase 3 build-tag variants; golden-diff vs `bazel build --config=minimal` |
| CGO cross | blst SIMD / libp2p link errors per target | validate each GOOS/GOARCH early; reuse Zig like Bazel did |
| nogo parity | losing a custom check silently | multichecker embeds the *same* analyzers; CI gate before deleting nogo |
| Reproducible/stamped builds | `-trimpath`/ldflags differences | add `-trimpath`, fixed ldflags; compare binary metadata |

## Suggested sequencing
Phases 1–3 first (unblocks pure-Go build incl. minimal), then 7 (lint gate), then 4–6
(release artifacts), then 8–9 (CI), then 10 (delete). Phases 4/5/6 and 7 are largely
parallelizable once 1–3 land.
