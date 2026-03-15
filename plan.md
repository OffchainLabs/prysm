# Bazel Usage Audit in Prysm — Complete Inventory

## Context
Goal: Remove Bazel entirely from the prysm repo and replace with standard Go tooling. This document catalogs every place Bazel is used so we can plan the removal systematically.

---

## 1. BUILD Files (467 files)
- **464 `BUILD.bazel`** files spread across every Go package
- **3 `BUILD`** files in `tools/cross-toolchain/configs/`
- These define `go_library`, `go_test`, `go_binary`, `go_proto_library`, `ssz_gen_marshal`, container image rules, etc.
- **Action**: Delete all 467 files. Standard `go build`/`go test` replaces them.

## 2. Workspace & Module Files
- `WORKSPACE` — root workspace defining all external deps (rules_go, gazelle, rules_oci, rules_proto, hermetic_cc_toolchain, consensus spec test archives, etc.)
- `WORKSPACE` in `tools/cross-toolchain/configs/cc/`
- `MODULE.bazel` + `MODULE.bazel.lock`
- **Action**: Delete all.

## 3. Bazel Config Files
- `.bazelrc` — root config importing sub-configs
- `.bazelversion` (7.4.1)
- `.buildkite-bazelrc` — CI remote cache config (BuildBuddy)
- `build/bazelrc/` directory (6 files): `convenience.bazelrc`, `correctness.bazelrc`, `cross.bazelrc`, `debug.bazelrc`, `hermetic-cc.bazelrc`, `performance.bazelrc`
- **Action**: Delete all.

## 4. Starlark Rule Files (.bzl) — 17 files
- `deps.bzl` — **803 `go_repository` rules** managed by gazelle (mirrors go.mod/go.sum)
- `distroless_deps.bzl` — OCI image deps
- `proto/ssz_proto_library.bzl` — custom SSZ proto templating (mainnet/minimal size substitutions)
- `tools/ssz.bzl` — `ssz_gen_marshal` rule for SSZ codegen
- `tools/go/def.bzl` — custom wrappers around `go_library`/`go_test` with network-specific build tag transitions
- `tools/build_settings.bzl`, `tools/download_spectests.bzl`, `tools/multi_arch.bzl`, `tools/prysm_image.bzl`, `tools/image_deps.bzl`
- `tools/nogo_config/def.bzl` — nogo analyzer config
- `tools/cross-toolchain/` — 5 `.bzl` files + 3 `.bzl.tpl` templates for cross-compilation toolchains
- `third_party/herumi/herumi.bzl`
- **Action**: Delete all. SSZ/proto codegen needs replacement scripts (see section 9).

## 5. Go Source Files With Bazel Integration

### 5a. `build/bazel/` package (4 files) — Bazel abstraction layer
- `bazel.go` (`//go:build bazel`) — wraps `rules_go/go/tools/bazel` for Runfile, TestTmpDir, FindBinary, SetGoEnv
- `non_bazel.go` (`//go:build !bazel`) — stubs that panic
- `data_path.go` — `TestDataPath()` uses `BuiltWithBazel()` to find testdata via runfiles or relative path
- `bazel_test.go`
- **Action**: Collapse to non-bazel implementations. `TestDataPath` already works without bazel (falls back to relative path). Remove `bazel.go`, make `non_bazel.go` stubs return sensible defaults instead of panicking, or just inline the non-bazel logic.

### 5b. `testing/util/bazel.go` — Bazel-aware test data access
- `BazelFileBytes`, `BazelListFiles`, `BazelListDirectories`, `BazelDirectoryNonEmpty`
- Directly imports `github.com/bazelbuild/rules_go/go/tools/bazel`
- **Used by 84 files** (mostly spec tests, e2e tests, config tests)
- **Action**: Replace with standard `os` package equivalents. These functions just resolve runfile paths; without bazel the paths are already relative to the package.

### 5c. `testing/bls/utils/utils.go`
- Imports `github.com/bazelbuild/rules_go/go/tools/bazel` for `Runfile()`
- **Action**: Replace with standard path resolution.

### 5d. Files importing `build/bazel` package (57 files calling `bazel.TestDataPath` etc.)
- Spec tests across all phases (phase0, altair, bellatrix, capella, deneb, electra, fulu, gloas)
- E2E test components (beacon_node, validator, boot_node, lighthouse, eth1, web3remotesigner)
- Config param tests, benchmark pregen, analyzer tests
- **Action**: After fixing the `build/bazel` package, these just work. No individual changes needed.

## 6. Shell Scripts Referencing Bazel
- `bazel.sh` — wrapper script to invoke bazel with controlled env
- `hack/check_gazelle.sh` — runs gazelle for dep sync checking
- `hack/update-go-pbs.sh` — builds protos via `bazel build`, copies from `bazel-bin`
- `hack/update-go-ssz.sh` — builds SSZ via `bazel build`, copies from `bazel-bin`
- `hack/ci-coverage.sh` — runs `bazel coverage`
- `hack/spectest-report.sh` — runs `bazel test //testing/spectest/...`
- `hack/build_and_upload_docker.sh` — uses `bazel build`/`bazel run` for OCI images
- `hack/gen-logs.sh` — excludes bazel output dirs
- **Action**: Rewrite to use `go build`/`go test`/`go generate`. Proto and SSZ generation scripts need `protoc` + `protoc-gen-go` directly. Docker builds need `docker build` with Dockerfiles.

## 7. CI/CD Configuration
- `.github/workflows/go.yml` — line 89 comment: "Tests run via Bazel for now..." (tests are NOT run in GH Actions, only build+lint)
- `.buildkite-bazelrc` — BuildBuddy remote cache config for Buildkite CI
- `.policy.yml` — approval rules matching `*.bazel` files
- **Action**: Add `go test ./...` to GH Actions workflow. Remove `.buildkite-bazelrc`. Update `.policy.yml`.

## 8. Documentation Referencing Bazel
- `DEPENDENCIES.md`, `INTEROP.md`, `third_party/README.md`, `third_party/blst/README.md`
- `tools/pcli/README.md`, `tools/cross-toolchain/README.md`, `testing/README.md`, `testing/spectest/README.md`, `testing/benchmark/README.md`
- `tools/specs-checker/README.md`, `tools/analyzers/README.md`, `tools/unencrypted-keys-gen/README.md`, `tools/enr-calculator/README.md`
- `CHANGELOG.md` (historical references)
- `hack/README.md`
- **Action**: Update all docs to reference `go build`/`go test`/`go run` instead.

## 9. Critical Bazel-Managed Functionality Needing Replacements

### 9a. Protobuf Compilation
- Currently: `go_proto_library` rules in `proto/*/BUILD.bazel` with custom protoc plugin `protoc-gen-go-cast`
- **Replacement**: Makefile/script using `protoc` + `protoc-gen-go` + `protoc-gen-go-grpc` (or `buf`)

### 9b. SSZ Code Generation
- Currently: `ssz_gen_marshal` bazel rule using `@com_github_prysmaticlabs_fastssz//sszgen`
- Network-specific size/type substitutions (mainnet vs minimal) via `ssz_proto_library.bzl`
- **Replacement**: `go generate` directives or a Makefile target calling `sszgen` directly

### 9c. Cross-Compilation
- Currently: Bazel cross-toolchain configs for linux_amd64, linux_arm64, osx_amd64, osx_arm64, windows_amd64
- **Replacement**: `GOOS`/`GOARCH` env vars with `go build`, plus CGO cross-compilation setup

### 9d. Container Image Building
- Currently: `rules_oci` with `prysm_image.bzl` for multi-arch OCI images
- **Replacement**: Multi-stage Dockerfiles + `docker buildx` for multi-arch

### 9e. Spec Test Data Download
- Currently: `http_archive` rules in WORKSPACE for consensus spec test vectors
- **Replacement**: Script to download/extract test fixtures, or `go generate` + `//go:embed`

### 9f. E2E Test Binary Dependencies
- Currently: `testing/endtoend/deps.bzl` downloads Lighthouse + Web3Signer binaries
- **Replacement**: Script to download binaries, or check them in, or use `go generate`

### 9g. Nogo Static Analysis
- Currently: Custom `nogo` rule in root BUILD.bazel with staticcheck analyzers
- **Replacement**: `golangci-lint` (already configured in CI via `.golangci.yml`)

### 9h. Gazelle Dependency Sync
- Currently: `gazelle` keeps `deps.bzl` (803 go_repository rules) in sync with `go.mod`
- **Replacement**: Not needed — `go.mod`/`go.sum` is the sole source of truth

## 10. `.gitignore` Entry
- `bazel-*` pattern ignoring bazel output symlinks
- **Action**: Remove after cleanup.

## 11. Go Module Dependency on rules_go
- `go.mod` / `go.sum` contain `github.com/bazelbuild/rules_go`
- **Action**: Remove after all Go code stops importing it.

---

## Summary: Files to Delete
| Category | Count |
|---|---|
| BUILD / BUILD.bazel | 467 |
| WORKSPACE files | 2 |
| MODULE.bazel files | 2 |
| .bazelrc / .bazelversion / .buildkite-bazelrc | 9 |
| .bzl files | 17 |
| .bzl.tpl templates | 3 |
| Cross-toolchain support files | ~10 |
| `bazel.sh` wrapper | 1 |
| **Total files to delete** | **~511** |

## Summary: Files to Modify
| Category | Count |
|---|---|
| `build/bazel/` Go package | 4 files (rewrite/simplify) |
| `testing/util/bazel.go` | 1 file (rewrite) |
| `testing/bls/utils/utils.go` | 1 file (remove bazel import) |
| Shell scripts in `hack/` | 7 scripts (rewrite) |
| CI workflows | 2 files |
| Documentation | ~15 files |
| `.gitignore` | 1 file |
| `.policy.yml` | 1 file |
| `go.mod` / `go.sum` | remove rules_go dep |
| **Total files to modify** | **~32+** |
