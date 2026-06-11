# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------
SHELL := /bin/bash
.SHELLFLAGS := -o pipefail -c

GO         ?= go
DIST       ?= dist
VERSION_PKG := github.com/OffchainLabs/prysm/v7/runtime/version

# Binaries built by `make build` (mode=dev or mode=release): every main package under cmd/,
# auto-discovered from cmd/*/main.go (a pure-make wildcard, no subprocess). The output
# name is the directory name and the package path is ./cmd/<name>, so there is no list
# to maintain — a new cmd/<tool>/main.go is picked up automatically. Non-cmd mains (e.g.
# the bootnode tool under tools/) are not built here; use `make build-tools` for those.
BINARIES := $(notdir $(patsubst %/,%,$(dir $(wildcard cmd/*/main.go))))

# Code-generation kinds (for `make gen [proto|ssz|mocks]`) -> the script each one runs.
GEN_KINDS := proto ssz mocks
GEN_SCRIPT_proto := ./hack/update-go-pbs.sh
GEN_SCRIPT_ssz   := ./hack/update-go-ssz.sh
GEN_SCRIPT_mocks := ./hack/update-mockgen.sh

# Cross-compilation (Phase 4). The set of binaries DISTRIBUTED via prysm.sh/prysm.bat
# (prysmaticlabs.com/releases), built by `make build platforms=all`. All live at ./cmd/<name>.
# This currently matches the full cmd/ set ($(BINARIES)) but is kept an explicit list, since
# "built" and "distributed" are distinct concerns (a new cmd/ tool would be built by
# `make build` but not auto-shipped).
CROSS_BINARIES := beacon-chain validator client-stats prysmctl

# Run-targets as "<goos>/<goarch>/<c-target-triple>" for `make build platforms=all` (and the
# docker image build). The C toolchain is chosen per-OS by build/cross, mirroring exactly what
# Bazel used — all five targets build from a single Linux x86_64 host, and only Linux was ever
# hermetic:
#   linux   -> `zig cc` (hermetic, any host; triple selects glibc 2.31, Bazel's baseline)
#   darwin  -> osxcross o64-clang/oa64-clang (Linux->macOS, embeds MacOSX12.3 SDK). Needed
#              because herumi's prebuilt C++ lib + the prometheus/mach cgo require Apple's SDK.
#   windows -> mingw-w64 (`x86_64-w64-mingw32-gcc`); zig's windows-gnu can't resolve the
#              libstdc++ symbols in herumi's MinGW-built prebuilt lib (crypto/bls/herumi)
# `make build platforms=all` auto-provisions every toolchain it needs (no manual setup): zig via
# install-zig.sh, mingw-w64 via install-mingw.sh, osxcross via install-osxcross.sh. Each is
# idempotent (no-op if already present); the package-manager and osxcross steps use sudo when
# not root. The darwin (osxcross) and windows (mingw) targets require a Linux x86_64 host; the
# linux targets use zig and build from any host (so `make build docker=true` works from macOS too).
CROSS_TARGETS := \
	linux/amd64/x86_64-linux-gnu.2.31 \
	linux/arm64/aarch64-linux-gnu.2.31 \
	darwin/amd64/x86_64-macos \
	darwin/arm64/aarch64-macos \
	windows/amd64/x86_64-windows-gnu

# linux/arm64 C optimization flags, ported from build/bazelrc/cross.bazelrc:32-37.
# -march=armv8-a is dropped: it is the aarch64 baseline already, and `zig cc` rejects the
# bare CPU name ("unknown CPU: 'armv8'").
CGO_CFLAGS_LINUX_ARM64 := -ftree-vectorize -funsafe-math-optimizations -fomit-frame-pointer

# blst (Prysm's only CGO dep) defaults to ADX/modern on amd64 via upstream's
# `#cgo amd64 CFLAGS: -D__ADX__`. -D__BLST_PORTABLE__ forces the portable path, matching
# Bazel's shipped default; the modern beacon-chain artifact omits it (amd64 only — ADX is x86).
BLST_PORTABLE := -D__BLST_PORTABLE__

# Version stamping: replaces Bazel --stamp / hack/workspace_status.sh, setting the
# same runtime/version vars its x_defs did. gitTag uses --abbrev=0 to get a clean
# vX.Y.Z (it feeds version.SemanticVersion()/BuildData()), matching workspace_status.sh.
GIT_COMMIT      := $(shell git rev-parse HEAD 2>/dev/null)
GIT_TAG         := $(shell git describe --tags --abbrev=0 2>/dev/null || echo Unknown)
BUILD_DATE      := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
BUILD_DATE_UNIX := $(shell date -u +%s)
# Dev builds (mode=dev) stamp only the stable commit/tag, so a repeat build hits Go's link
# cache instead of relinking — no volatile timestamp in the ldflags. The release paths
# (mode=release, in any of build's outputs) use LDFLAGS_STAMPED, which adds the wall-clock date.
LDFLAGS := -X $(VERSION_PKG).gitCommit=$(GIT_COMMIT) \
           -X $(VERSION_PKG).gitTag=$(GIT_TAG)
LDFLAGS_STAMPED := $(LDFLAGS) \
           -X $(VERSION_PKG).buildDate=$(BUILD_DATE) \
           -X $(VERSION_PKG).buildDateUnix=$(BUILD_DATE_UNIX)

TAGS ?=
TAGFLAG := $(if $(TAGS),-tags=$(TAGS),)

comma := ,
TEST_TAGS := develop$(if $(TAGS),$(comma)$(TAGS),)
TEST_TAGFLAG := -tags=$(TEST_TAGS)

# ---------------------------------------------------------------------------
# Help (default target)
# ---------------------------------------------------------------------------
.DEFAULT_GOAL := help
.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z0-9_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Run a subset by naming the part(s); omit to do all (e.g. 'make build beacon-chain', 'make gen proto'):"
	@printf "  \033[36m%-14s\033[0m %s\n" "build"       "$(BINARIES)"
	@printf "  \033[36m%-14s\033[0m %s\n" "gen"         "$(GEN_KINDS)"
	@printf "  \033[36m%-14s\033[0m %s\n" "test"        "$(TEST_KINDS) [mode=no-race|race] (default: no-race)"
	@printf "  \033[36m%-14s\033[0m %s\n" "e2e"         "$(E2E_KINDS) (default: minimal)"
	@echo ""
	@echo "build flags (make variables, since make can't take --flags; default in parens):"
	@printf "  \033[36m%-20s\033[0m %s\n" "mode=dev|release"   "(default: dev) optimized/stamped/stripped/PGO'd output"
	@printf "  \033[36m%-20s\033[0m %s\n" "docker=true|false"  "(default: false) build an OCI image instead of a host binary"
	@printf "  \033[36m%-20s\033[0m %s\n" "push=true|false"    "(default: false) push the multi-arch image (implies docker=true)"
	@printf "  \033[36m%-20s\033[0m %s\n" "platforms=host|all" "(default: host) cross-compile all release targets (implies docker=false)"

# ---------------------------------------------------------------------------
# Build & test
# ---------------------------------------------------------------------------
# `make build [TARGET...]` is the single build command. Behaviour is set by flag variables
# (make can't take --flags — see `make help` for the legend). Defaults: mode=dev docker=false
# push=false platforms=host. It subsumes the old cross-build / docker-build / docker-push:
#   docker=false platforms=host : native host binaries          -> dist/
#   docker=false platforms=all  : cross-compile all 5 targets   -> dist/        (build/cross)
#   docker=true                 : OCI image, local --load (host arch)           (build/docker)
#   docker=true  push=true      : OCI image, multi-arch amd64+arm64 --push      (build/docker)
# mode applies to every path: release = stamped + stripped + PGO'd; dev = fast, unstripped.

# PGO profile for beacon-chain (release only; the sole binary with a committed profile).
# Consumed by every release path (native loop below, and build/cross|docker via BUILD_PGO).
PGO_beacon-chain := -pgo=cmd/beacon-chain/pprof.beacon-chain.samples.cpu.pb.gz

# --- flags + validation -----------------------------------------------------------------
docker    ?= false
push      ?= false
platforms ?= host
# mode is per-verb (test uses no-race|race; build uses dev|release), with its own default.
ifneq ($(filter test,$(MAKECMDGOALS)),)
VALID_MODES := no-race race
mode ?= no-race
else
VALID_MODES := dev release
mode ?= dev
endif
ifeq ($(filter $(mode),$(VALID_MODES)),)
$(error invalid mode '$(mode)' — for this target use one of: $(VALID_MODES))
endif
ifeq ($(filter $(docker),true false),)
$(error invalid docker '$(docker)' — use docker=true|false)
endif
ifeq ($(filter $(push),true false),)
$(error invalid push '$(push)' — use push=true|false)
endif
ifeq ($(filter $(platforms),host all),)
$(error invalid platforms '$(platforms)' — use platforms=host|all)
endif
ifeq ($(push)$(docker),truefalse)
$(error push=true requires docker=true)
endif
ifeq ($(platforms)$(docker),alltrue)
$(error platforms applies to native binary builds only (docker=false); it has no effect on docker images, which are linux-only)
endif

RELEASE       := $(filter release,$(mode))
BUILD_LDFLAGS := $(if $(RELEASE),$(LDFLAGS_STAMPED) -s -w,$(LDFLAGS))
BUILD_PGO     := $(if $(RELEASE),$(PGO_beacon-chain),)
# dev|release label for the progress line (BUILD_CROSS_ENV passes it to the cross/docker
# helpers so every path prints the same "(mode - portable|modern)" suffix).
BUILD_MODE    := $(if $(RELEASE),release,dev)

# Host platform + blst variant for the native build's progress line, so it matches the
# cross path's "[n/m] → os/arch  bin  (mode - portable|modern)" format. One `go env` call
# (two values). Native builds pass no CGO_CFLAGS, so blst takes upstream's amd64 ADX default
# (modern); every other arch is portable (ADX is x86-only — see BLST_PORTABLE above).
HOST_PLATFORM := $(shell $(GO) env GOHOSTOS GOHOSTARCH)
HOST_OS       := $(word 1,$(HOST_PLATFORM))
HOST_ARCH     := $(word 2,$(HOST_PLATFORM))
HOST_BLST     := $(if $(filter amd64,$(HOST_ARCH)),modern,portable)

# Active binary set per output, and the named-subset / bad-token selection from the goals
# (consolidates the old DOCKER_/CROSS_ guards). docker -> image set; platforms=all ->
# distributed set; otherwise the cmd/ set. (recursive '=': $(DOCKER_BINARIES)/$(POSITIONAL)
# are defined just below.)
ifeq ($(docker),true)
BUILD_SET = $(DOCKER_BINARIES)
else ifeq ($(platforms),all)
BUILD_SET = $(CROSS_BINARIES)
else
BUILD_SET = $(BINARIES)
endif
BUILD_BINS = $(strip $(filter $(BUILD_SET),$(MAKECMDGOALS)))
BUILD_BAD  = $(strip $(filter-out $(BUILD_SET),$(filter $(POSITIONAL),$(MAKECMDGOALS))))

# Env for the cross/docker Go helpers — mode-aware (replaces the old always-stamped CROSS_ENV).
BUILD_CROSS_ENV = GO="$(GO)" DIST="$(DIST)" GIT_TAG="$(GIT_TAG)" \
	CGO_CFLAGS_LINUX_ARM64="$(CGO_CFLAGS_LINUX_ARM64)" BLST_PORTABLE="$(BLST_PORTABLE)" \
	LDFLAGS="$(BUILD_LDFLAGS)" TAGFLAG="$(TAGFLAG)" PGO_beacon_chain="$(BUILD_PGO)" \
	BUILD_MODE="$(BUILD_MODE)"

# Docker image config (used by `make build ... docker=true`). Faithful repos: beacon-chain,
# validator -> $(DOCKER_REGISTRY)/<bin>; prysmctl -> $(DOCKER_REGISTRY)/cmd/prysmctl.
DOCKER_REGISTRY     ?= gcr.io/offchainlabs/prysm
DOCKER_TAG          ?= $(GIT_TAG)
DOCKER_BINARIES     := beacon-chain validator prysmctl
CROSS_TARGETS_LINUX := $(filter linux/%,$(CROSS_TARGETS))

# Positional goals: `make build beacon-chain`, `make build prysmctl docker=true`, `make gen
# proto`. These tokens are phony no-op goals so make doesn't error on the extra word; each verb
# recipe reads $(MAKECMDGOALS) to pick what to do (none named -> all). beacon-chain/, validator/
# and proto/ are real dirs, so .PHONY is required.
POSITIONAL = $(sort $(BINARIES) $(CROSS_BINARIES) $(DOCKER_BINARIES) $(GEN_KINDS) $(TEST_KINDS) $(E2E_KINDS))
# The no-op rule that makes these tokens valid goals is defined near the bottom, AFTER
# TEST_KINDS / E2E_KINDS are set — a rule's target list is expanded when parsed, so those
# `:=` vars must already be defined or their tokens (minimal, mainnet, e2e kinds) would be
# silently dropped from the rule.

.PHONY: build
build:
	@$(if $(BUILD_BAD),echo "❌ build: not buildable here: $(BUILD_BAD) (available: $(BUILD_SET))" >&2; exit 1;) \
	bins="$(or $(BUILD_BINS),$(BUILD_SET))"; \
	if [ "$(docker)" = true ]; then \
	  if [ "$(push)" = true ]; then M=push; else M=load; fi; \
	  $(BUILD_CROSS_ENV) MODE=$$M TAG="$(DOCKER_TAG)" REGISTRY="$(DOCKER_REGISTRY)" \
	    DOCKER_BINARIES="$$bins" CROSS_TARGETS_LINUX="$(CROSS_TARGETS_LINUX)" \
	    $(GO) run ./build/docker; \
	elif [ "$(platforms)" = all ]; then \
	  $(BUILD_CROSS_ENV) CROSS_BINARIES="$$bins" CROSS_TARGETS="$(CROSS_TARGETS)" \
	    $(GO) run ./build/cross; \
	else \
	  mkdir -p $(DIST); \
	  m=$$(set -- $$bins; echo $$#); n=0; \
	  for b in $$bins; do \
	    n=$$((n + 1)); \
	    pgo=""; $(if $(RELEASE),[ "$$b" = beacon-chain ] && pgo="$(PGO_beacon-chain)";) \
	    echo "[$$n/$$m] → $(HOST_OS)/$(HOST_ARCH)  $$b  ($(mode) - $(HOST_BLST))"; \
	    $(GO) build $(TAGFLAG) -trimpath $$pgo -ldflags "$(BUILD_LDFLAGS)" -o "$(DIST)/$$b" "./cmd/$$b" || exit 1; \
	  done; \
	  echo "✅ build: built $$n/$$m binaries → $(DIST)/"; \
	fi

# build-tools builds every tool (each `package main` under tools/) into $(DIST)/,
# discovered via `go list` so there is no list to maintain. Output names are the
# package basenames (all unique). The cmd/ primaries are built by `make build`; the
# build/ packages (cross, docker, test — this build system itself) live outside tools/
# and are run via `go run`, so neither is included here.
.PHONY: build-tools
build-tools: ## Build every tool (main packages under tools/) into $(DIST)/
	@mkdir -p $(DIST)
	@for p in $$($(GO) list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./tools/...); do \
		echo "building $$(basename $$p)"; \
		$(GO) build $(TAGFLAG) -trimpath -ldflags "$(LDFLAGS)" -o "$(DIST)/$$(basename $$p)" "$$p" || exit 1; \
	done

.PHONY: testdata
testdata: ## Pre-fetch all external spec-test data (tests fetch lazily otherwise)
	$(GO) run ./tools/cmd/fetch-testdata

# Mainnet pass excludes E2E (Phase 8) and the minimal-config packages — the
# latter run in the separate minimal pass below. The package enumeration/filtering
# now lives in build/test (which reads this regexp via TEST_EXCLUDE).
TEST_EXCLUDE := /testing/endtoend|/testing/spectest/minimal|/beacon-chain/rpc/prysm/v1alpha1/beacon$$|/beacon-chain/rpc/prysm/v1alpha1/validator$$

# Packages tested under -tags=minimal (minimal consensus config): the minimal spec
# tests, the beacon + validator rpc packages (eth_network=minimal in Bazel), and the
# minimal fieldparams test. fieldparams also runs in the mainnet pass (mainnet_test.go).
# validator is a minimal-config package (its TestMain sets minimal config), so it runs
# only here; its few mainnet-only tests are tagged //go:build !minimal to keep them out
# of the minimal build (they aren't run in either pass, as before Phase 3 — running them
# would need a dedicated mainnet harness).
MINIMAL_PKGS := ./testing/spectest/minimal/... ./beacon-chain/rpc/prysm/v1alpha1/beacon ./beacon-chain/rpc/prysm/v1alpha1/validator ./config/fieldparams
MINIMAL_TAGFLAG := -tags=$(TEST_TAGS),minimal

# The two `make test` passes (`make test mainnet` / `minimal` runs just one; none -> both).
TEST_KINDS := mainnet minimal

# E2E scenarios for `make e2e [kind|suite]` (default: minimal). build/e2e maps each kind to
# a Go test func, builds the binaries it launches (+ geth, and lighthouse/web3signer where
# needed), and runs `go test ./testing/endtoend`. The trailing suites (presubmit/postsubmit/
# scenario_tests) run the same bundles the Bazel test_suites did, in sequence.
# /testing/endtoend stays excluded from `make test` (it's heavy and launches a local devnet).
E2E_KINDS := minimal builder web3signer slasher slashing scenario postmerge statediff mainnet multiclient \
             presubmit postsubmit scenario_tests

# Positional no-op goals (see POSITIONAL above) — defined here so TEST_KINDS/E2E_KINDS are
# already set when this rule's target list is expanded.
.PHONY: $(POSITIONAL)
$(POSITIONAL):
	@:

# gotestsum (pinned via the go.mod tool directive) wraps `go test`, reformats
# output, and reruns flaky failures up to 5 times — matching Bazel's
# --flaky_test_attempts=5. If more than RERUN_MAX distinct tests fail it's a real
# breakage, not flakiness, so reruns are skipped.
RERUN_ATTEMPTS ?= 5
RERUN_MAX ?= 1000
# --no-color=false forces color even though build/test pipes gotestsum into its
# [X/N] progress counter (a pipe is not a TTY, so gotestsum would otherwise disable
# color). --hide-summary=skipped drops the per-skipped-test "=== SKIP:" list from
# the end-of-run summary (spec tests skip thousands of "unused type" cases);
# failures and errors are still summarized.
GOTESTSUM_FLAGS := --format=pkgname --no-color=false --hide-summary=skipped --rerun-fails=$(RERUN_ATTEMPTS) --rerun-fails-max-failures=$(RERUN_MAX)

# Shared environment for build/test (the gotestsum runner): which packages each pass
# covers, the build-tag flags, and gotestsum's flags. The pass dispatch, single
# `go list`, and progress counter all live in build/test.
TEST_ENV = GO="$(GO)" TEST_EXCLUDE='$(TEST_EXCLUDE)' MINIMAL_PKGS='$(MINIMAL_PKGS)' \
	TEST_TAGFLAG='$(TEST_TAGFLAG)' MINIMAL_TAGFLAG='$(MINIMAL_TAGFLAG)' \
	GOTESTSUM_FLAGS='$(GOTESTSUM_FLAGS)' RERUN_ATTEMPTS='$(RERUN_ATTEMPTS)'

# Run unit tests — both passes ($(TEST_KINDS)), or only those named (e.g. make test minimal).
# mode=race adds the race detector (e.g. `make test mainnet mode=race`).
.PHONY: test
test:
	@$(TEST_ENV) $(GO) run ./build/test $(if $(filter race,$(mode)),-race,) $(filter $(POSITIONAL),$(MAKECMDGOALS))

# ---------------------------------------------------------------------------
# gen / lint / deb
# ---------------------------------------------------------------------------
.PHONY: gen lint

# Regenerate generated code — all of $(GEN_KINDS), or only those named (e.g. make gen proto).
gen:
	@kinds="$(strip $(filter $(GEN_KINDS),$(MAKECMDGOALS)))"; [ -n "$$kinds" ] || kinds="$(GEN_KINDS)"; \
	for k in $$kinds; do \
	  script=""; $(foreach g,$(GEN_KINDS),if [ "$$k" = "$(g)" ]; then script="$(GEN_SCRIPT_$(g))"; fi;) \
	  echo "==> gen $$k"; "$$script" || exit 1; \
	done

# Static analysis (replaces Bazel's nogo): tools/cmd/prysm-vet is a multichecker
# embedding the same analyzer set (custom + x/tools passes + staticcheck SA*, minus
# SA1019) and reproducing nogo's per-analyzer file exclusions from nogo_config.json.
# golangci-lint and gosec remain separate CI steps (see .github/workflows). To lint a
# subset, call the binary directly: `go run ./tools/cmd/prysm-vet ./beacon-chain/...`.
lint: ## [Phase 7] Static analysis (nogo → prysm-vet multichecker)
	@$(GO) run ./tools/cmd/prysm-vet ./...

# Build the .deb packages (prysm-beacon-chain, prysm-validator) with nfpm. Replaces the
# Bazel rules_pkg pkg_deb rules. build/deb cross-builds the linux portable binaries
# in-process (reusing build/cross via BUILD_CROSS_ENV) then runs `go tool nfpm` per arch.
# Contrary to Bazel (amd64 only) we ship both amd64 and arm64, matching the docker images.
# Turnkey on any host: the linux targets build via zig (like `make build docker=true`).
.PHONY: deb
deb: ## [Phase 6] Build .deb packages (prysm-beacon-chain, prysm-validator; amd64+arm64)
	@$(BUILD_CROSS_ENV) DEB_ARCHES="amd64 arm64" \
	  CROSS_TARGETS_LINUX="$(CROSS_TARGETS_LINUX)" $(GO) run ./build/deb

# End-to-end tests (Phase 8). build/e2e builds the launched binaries (beacon-chain,
# validator, bootnode) + geth into $(DIST), provisions lighthouse/web3signer when the
# selected scenario needs them, and runs `go test ./testing/endtoend` with PRYSM_BIN
# pointed at $(DIST). Name a scenario to pick it; default is the minimal single-client run.
.PHONY: e2e
e2e: ## [Phase 8] End-to-end tests (name a scenario; default minimal — see list below)
	@GO="$(GO)" DIST="$(DIST)" $(GO) run ./build/e2e $(filter $(E2E_KINDS),$(MAKECMDGOALS))

.PHONY: clean
clean: ## Remove build output
	rm -rf $(DIST)

