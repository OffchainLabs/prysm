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

### Phase 4 — CGO cross-compilation — ✅ DONE (windows pending mingw on the release host)
Implemented as `make build platforms=all` (Makefile) + `tools/cross-toolchain/install-zig.sh`. The five
distributed run-targets (linux/{amd64,arm64}, darwin/{amd64,arm64}, windows/amd64) for the
shipped binary set (beacon-chain, validator, client-stats, prysmctl — **not** bootnode),
matching the names `prysm.sh`/`prysm.bat` fetch.

**CGO surface (discovered):** Prysm's cgo deps are **blst** (C), **herumi
`bls-eth-go-binary`** (prebuilt C++ static libs, imported *unconditionally* by
`crypto/bls/bls.go` "temporarily while we transition to blst for ethdo"), and a small
**prometheus/client_golang** darwin mach cgo file. `gohashtree` is **not** cgo (pure Go +
Go-asm). The herumi C++ libs and the darwin mach headers are what break full hermeticity.

**Build host: a single Linux x86_64 machine builds all five targets** — exactly as Bazel
did (its RBE worker was `debian:bullseye-slim` with osxcross + mingw-w64; `cross.bazelrc:42`
"all docker sandbox configs must run from a linux x86_64 host").

**`make build platforms=all` is turnkey — it auto-provisions every toolchain it needs** (no manual setup),
each via an idempotent script under `tools/cross-toolchain/`:
- `install-zig.sh` — pinned zig (user cache, no root)
- `install-mingw.sh` — gcc + g++ mingw-w64 (POSIX threads) via apt/dnf/pacman
- `install-osxcross.sh` — build deps + osxcross (delegates to the unchanged
  `install_osxcross.sh` + `link_osxcross.sh`, MacOSX12.3 SDK)
The package-manager and osxcross steps use `sudo` when not already root. **Host requirement is
per-target**: the **darwin** (osxcross) and **windows** (mingw-w64) targets require a Linux
x86_64 host; the **linux** targets use zig and build from any host — so `make build platforms=all` for
linux targets, and therefore `make build docker=true` (linux images only), also run on macOS. For CI, baking
mingw-w64 + osxcross into the build image (as Bazel's RBE worker did) avoids the per-run install.

**Per-OS toolchain** (mirrors what Bazel actually used — only Linux was hermetic):
```
linux   amd64  CC="zig cc -target x86_64-linux-gnu.2.31"
linux   arm64  CC="zig cc -target aarch64-linux-gnu.2.31"   CGO_CFLAGS+=-ftree-vectorize -funsafe-math-optimizations -fomit-frame-pointer
darwin  amd64  CC="o64-clang"              # osxcross (Linux->macOS), embeds MacOSX12.3 SDK
darwin  arm64  CC="oa64-clang"             # osxcross (Linux->macOS), embeds MacOSX12.3 SDK
windows amd64  CC="x86_64-w64-mingw32-gcc" # mingw-w64; zig's windows-gnu can't link herumi's libstdc++
```
Notes / deviations from the original "zig for everything" sketch:
- **zig pinned to 0.14.1** (not hermetic_cc_toolchain's 3.0.1 numbering — that's the Bazel
  rule version; we pin the zig *compiler*). `install-zig.sh` downloads+verifies it per host.
- **glibc 2.31** pinned via the `.2.31` triple suffix (Bazel's Ubuntu-20.04 baseline).
- `-march=armv8-a` **dropped** — it is the aarch64 baseline and `zig cc` rejects the name.
- **darwin is not hermetic and is not zig**: zig doesn't ship Apple's SDK and can't consume
  the SDK header layout even with `-isysroot`. We keep Bazel's osxcross path unchanged
  (`tools/cross-toolchain/{common,install,link}_osxcross.sh`, SDK 12.3) and the Makefile
  invokes its `o64-clang`/`oa64-clang` wrappers. A macOS host with native `clang -arch` also
  works for local dev, but the faithful/CI path is osxcross-on-Linux.
- **windows needs mingw-w64**, not zig: herumi's Windows lib is MinGW/libstdc++-built and
  zig's `windows-gnu` provides libc++, leaving libstdc++ symbols undefined.
- **blst variant:** `-D__BLST_PORTABLE__` is passed by default (matches Bazel's shipped
  portable images; upstream's Go bindings otherwise default to `-D__ADX__` on amd64). The
  `beacon-chain-<tag>-modern-<os>-amd64` artifact omits it (ADX is x86-only). Verified: the
  modern build SIGILLs on a non-ADX CPU while portable runs — proof the flag takes effect.
- **Path to full hermeticity:** removing the unconditional `crypto/bls/herumi` import (once
  the ethdo transition completes) would let windows build under zig and drop the mingw
  dependency; darwin would still need osxcross/macOS for the prometheus mach cgo.

### Phase 5 — Docker / OCI images — ✅ DONE
Replaced `rules_oci` + `prysm_image.bzl` with a single parameterized **multi-stage Dockerfile
+ `docker buildx`** (`tools/docker/Dockerfile`), driven by `make build docker=true [push=true]`
(the `build/docker` Go command). Images embed the portable binaries from Phase 4's `build/cross`.

- **Faithful base reproduction** (user's choice): final stage `FROM` the pinned
  `gcr.io/prysmaticlabs/distroless/cc-debian11@sha256:55a5…`, with a `rootfs` builder stage
  (`debian:bullseye-slim`) that downloads+verifies+extracts the exact Debian-11 userland
  (bash, coreutils + lib deps) via `tools/docker/debian-pkgs.sh` (a byte-faithful port of
  `tools/image_deps.bzl`), writes `/etc/passwd` (root+nonroot), and creates `/bin/sh`→`/bin/bash`,
  the `/app/cmd/<bin>/<bin>`→`/<bin>` backwards-compat symlink, and `/entrypoint`→`/<bin>`.
  (Fixed a latent bug in `image_deps.bzl`: the arm64 `libtinfo6` pool path said `deb11u1`,
  but the pinned sha and the only existing file are `deb11u2`.)
- **Images** (unchanged set): beacon-chain, validator, prysmctl (no client-stats).
- **Faithful tags** (user's choice = same as Bazel): beacon-chain → `$TAG` **and**
  `$TAG-portable` (both portable); validator → `$TAG`; prysmctl → `$TAG`. Repos:
  `gcr.io/offchainlabs/prysm/{beacon-chain,validator,cmd/prysmctl}`. Label
  `org.opencontainers.image.source` preserved.
- **`make build docker=true`** builds host-arch images and `--load`s them locally
  (`prysm/<bin>:<tag>`) for testing; **`make build docker=true push=true`** builds the
  linux/amd64+arm64 multi-arch manifest (the `oci_image_index` equivalent) and `--push`es it.
  The push auto-creates a `docker-container` buildx builder (the default `docker` driver can't do
  multi-arch). `mode=dev` (default) yields a fast unstripped image; `mode=release` a
  stamped/stripped/PGO'd one. Uses `tools/docker/Dockerfile`, which *assembles* a binary
  cross-compiled in-process by `build/crossbuild`. Replaces `hack/build_and_upload_docker.sh`
  (left for Phase 10 deletion).
- **Self-contained root `Dockerfile`** for one-command dev builds: `docker build . -t <name>`
  compiles the binary in-container (defaults to beacon-chain; `--build-arg BIN=validator|prysmctl`)
  *and* assembles the image — no `make build platforms=all`/`dist/` prerequisite. It compiles with the same
  pinned **zig** toolchain (portable blst, glibc-2.31 floor) so the binary matches what we ship,
  rather than the builder image's gcc (golang:1.25-bookworm is glibc 2.36, which would break the
  glibc-2.31 distroless base). Version stamping defaults to `gitTag=dev` (`--build-arg TAG=…` to set).
- **Verified**: amd64 image builds + runs (`--version` shows the stamped tag); `/bin/sh` shell
  works; `/beacon-chain`, `/app/cmd/beacon-chain/beacon-chain`, `/entrypoint` all resolve;
  passwd has root+nonroot; OCI source label + entrypoint correct; multi-arch OCI manifest
  contains both linux/amd64 and linux/arm64.
- Notes: entrypoint is `/entrypoint` (a symlink → `/<bin>`) since one Dockerfile can't put an
  `ARG` in `ENTRYPOINT`; the real binary stays at the faithful `/<bin>` path. `make build docker=true`
  builds only linux binaries (via zig), so it runs from any host including macOS.

### Phase 6 — `.deb` packaging — ✅ DONE
Replaced `rules_pkg` (`pkg_deb`/`pkg_tar`) with **`nfpm`** (pinned in the go.mod `tool`
block, invoked as `go tool nfpm`), driven by `make deb` (the `build/deb` Go command +
`build/debpkg` logic, mirroring the `build/docker`⇄`build/crossbuild` split).

- **Packages** (unchanged set): `prysm-beacon-chain`, `prysm-validator` — one `nfpm.yaml`
  each at `beacon-chain/package/` / `validator/package/`, reusing the existing config
  YAML, systemd unit, and `preinst.sh`/`postinst.sh` verbatim.
- **Faithful deb layout** (byte-checked against the old `pkg_deb`): `/usr/bin/<bin>`
  (0755), `/etc/prysm/<bin>.yaml` (0640, marked `type: config` → Debian conffile),
  `/usr/lib/systemd/system/prysm-<name>.service` (0640); same package name, description,
  maintainer (`Prysmatic Labs <contact@prysmaticlabs.com>`), homepage; no
  `depends`/`prerm`/`postrm`. **Version** = `GIT_TAG` with the leading `v` stripped
  (`version_schema: none` passes it through verbatim), matching the old
  `//runtime:version_file` genrule (`... | tr -d v`).
- **Contrary to Bazel (which shipped `amd64` only), we build both `amd64` and `arm64`**
  — matching the Phase 5 multi-arch docker images; Phase 4 already cross-builds both
  linux binaries, so it is free. Output: `dist/prysm-{beacon-chain,validator}_<ver>_{amd64,arm64}.deb`.
- **`make deb` is turnkey** — `build/deb` cross-builds the linux **portable** binaries
  in-process (reusing `build/crossbuild` via `BUILD_CROSS_ENV`, exactly as `build/docker`
  does), then runs `nfpm` per (package × arch). Runs from any host (the linux targets use
  zig), like `make build docker=true`.
- nfpm gotcha: a content entry's `src`/`dst` are env-expanded only with `expand: true` on
  that entry (`nfpm.go:197`); the binary entry uses `${PRYSM_BIN_SRC}` and sets it.
- The old `*/package/BUILD.bazel` + `rules_pkg` in `WORKSPACE` are left for Phase 10 deletion.

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
