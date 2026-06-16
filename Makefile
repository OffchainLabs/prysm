SHELL := /bin/bash
.SHELLFLAGS := -o pipefail -c

GO      ?= go
DIST    ?= dist
BIN_DIR ?= bin

empty :=
space := $(empty) $(empty)
comma := ,

BINARIES := $(notdir $(patsubst %/,%,$(dir $(wildcard cmd/*/main.go))))
GEN_KINDS := proto ssz mocks
TEST_KINDS := mainnet minimal

E2E_SCENARIOS := minimal builder web3signer slasher slashing scenario postmerge statediff mainnet multiclient
E2E_SUITES    := presubmit postsubmit scenario_tests
E2E_KINDS     := $(E2E_SCENARIOS) $(E2E_SUITES)
E2E_SUITE_presubmit      := minimal statediff slashing slasher
E2E_SUITE_postsubmit     := builder postmerge mainnet multiclient
E2E_SUITE_scenario_tests := scenario scenario-multiclient

POSITIONAL := $(sort $(GEN_KINDS) $(TEST_KINDS) $(E2E_KINDS) $(BINARIES))

VERSION_PKG := github.com/OffchainLabs/prysm/v7/runtime/version
GIT_COMMIT  := $(shell git rev-parse HEAD 2>/dev/null)
GIT_TAG     := $(shell git describe --tags --abbrev=0 2>/dev/null || echo Unknown)
LDFLAGS     := -X $(VERSION_PKG).gitCommit=$(GIT_COMMIT) \
               -X $(VERSION_PKG).gitTag=$(GIT_TAG)

CROSS_BINARIES := beacon-chain validator client-stats prysmctl
CROSS_TARGETS := \
	linux/amd64/x86_64-linux-gnu.2.31 \
	linux/arm64/aarch64-linux-gnu.2.31 \
	darwin/amd64/x86_64-macos \
	darwin/arm64/aarch64-macos \
	windows/amd64/x86_64-windows-gnu

CROSS_PLATFORMS := $(foreach t,$(CROSS_TARGETS),$(word 1,$(subst /,$(space),$(t)))/$(word 2,$(subst /,$(space),$(t))))

DOCKER_IMAGES  := beacon-chain validator prysmctl
DOCKER_REGISTRY ?= gcr.io/offchainlabs/prysm
DOCKER_TAG      ?= $(GIT_TAG)
LINUX_ARCHES   := $(foreach t,$(filter linux/%,$(CROSS_TARGETS)),$(word 2,$(subst /,$(space),$(t))))
DOCKER_PLATFORMS := $(foreach a,$(LINUX_ARCHES),docker/$(a))
ALL_PLATFORMS  := $(CROSS_PLATFORMS) $(DOCKER_PLATFORMS)

# `mode` selects the race setting and only applies to `make test` (`make test mode=race`).
TEST_MODES        := no-race race
MODE_DEFAULT_test := no-race
mode              ?= $(MODE_DEFAULT_test)

ifeq ($(filter $(mode),$(TEST_MODES)),)
$(error invalid mode '$(mode)' — only 'make test' takes a mode, one of: $(TEST_MODES))
endif

# ---------------------------------------------------------------------------
# Run a single binary on the host (sugar over `go run ./cmd/<bin>`)
# ---------------------------------------------------------------------------
# Binary is mandatory and positional; forward extra args after `--`:
#   make run beacon-chain -- --help            -> go run ./cmd/beacon-chain --help
# Caveat: make eats '='-tokens as variable assignments, so after `--` pass '--flag value'
# (not '--flag=value'); the catch-all `%` rule at the bottom absorbs those tokens as goals.
RUN_GOALS := $(filter-out run,$(MAKECMDGOALS))
RUN_BIN   := $(firstword $(RUN_GOALS))
RUN_ARGS  := $(wordlist 2,$(words $(RUN_GOALS)),$(RUN_GOALS))

.PHONY: run
run:
	@bin="$(RUN_BIN)"; \
	[ -n "$$bin" ] || { echo "❌ run: name a binary, e.g. 'make run beacon-chain -- --help' (one of: $(BINARIES))" >&2; exit 1; }; \
	case " $(BINARIES) " in *" $$bin "*) ;; *) echo "❌ run: '$$bin' is not a binary (one of: $(BINARIES))" >&2; exit 1;; esac; \
	exec $(GO) run "./cmd/$$bin" $(RUN_ARGS)

# ---------------------------------------------------------------------------
# Test
# ---------------------------------------------------------------------------
# The package sets, build tags, gotestsum flags and flaky-rerun budget all live in
# build/test (the single source of truth); the Makefile just dispatches to it.
.PHONY: test
test:
	@GO="$(GO)" $(GO) run ./build/test $(if $(filter race,$(mode)),-race,) $(filter $(POSITIONAL),$(MAKECMDGOALS))

# ---------------------------------------------------------------------------
# Build a single binary on the host (sugar over `go build ./cmd/<bin>`)
# ---------------------------------------------------------------------------
HOST_LDFLAGS := -ldflags "$(LDFLAGS)"
BUILD_GOALS  := $(filter-out build,$(MAKECMDGOALS))
BUILD_BIN    := $(firstword $(BUILD_GOALS))
BUILD_ARGS   := $(wordlist 2,$(words $(BUILD_GOALS)),$(BUILD_GOALS))

.PHONY: build
build:
	@bin="$(BUILD_BIN)"; \
	[ -n "$$bin" ] || { echo "❌ build: name a binary, e.g. 'make build beacon-chain' (one of: $(BINARIES))" >&2; exit 1; }; \
	case " $(BINARIES) " in *" $$bin "*) ;; *) echo "❌ build: '$$bin' is not a binary (one of: $(BINARIES))" >&2; exit 1;; esac; \
	mkdir -p $(BIN_DIR); \
	echo "→ go build ./cmd/$$bin -> $(BIN_DIR)/$$bin"; \
	$(GO) build $(HOST_LDFLAGS) -trimpath $(BUILD_ARGS) -o "$(BIN_DIR)/$$bin" "./cmd/$$bin"

# ---------------------------------------------------------------------------
# Code generation
# ---------------------------------------------------------------------------
GEN_GOALS := $(filter-out gen,$(MAKECMDGOALS))
GEN_BAD   := $(filter-out $(GEN_KINDS),$(GEN_GOALS))

.PHONY: gen
gen:
	@$(if $(GEN_BAD),echo "❌ gen: unknown kind(s): $(GEN_BAD)  (one of: $(GEN_KINDS))" >&2; exit 1;) \
	$(GO) run ./build/gen $(filter $(GEN_KINDS),$(MAKECMDGOALS))

# ---------------------------------------------------------------------------
# End-to-end tests
# ---------------------------------------------------------------------------
E2E_GOALS := $(filter-out e2e,$(MAKECMDGOALS))
E2E_BAD   := $(filter-out $(E2E_KINDS),$(E2E_GOALS))

.PHONY: e2e
e2e:
	@$(if $(E2E_BAD),echo "❌ e2e: unknown suite/scenario(s): $(E2E_BAD)  (one of: $(E2E_KINDS))" >&2; exit 1;) \
	GO="$(GO)" DIST="$(DIST)" $(GO) run ./build/e2e $(filter $(E2E_KINDS),$(MAKECMDGOALS))

# ---------------------------------------------------------------------------
# Distribution build (official, all-platforms, in Docker)
# ---------------------------------------------------------------------------
TAGS ?=
TAGFLAG := $(if $(TAGS),-tags=$(TAGS),)

# linux/arm64 C optimization flags (ported from Bazel's cross config).
CGO_CFLAGS_LINUX_ARM64 := -ftree-vectorize -funsafe-math-optimizations -fomit-frame-pointer

# blst (Prysm's CGO dep) defaults to ADX/modern on amd64; force the portable path. The
# modern amd64 beacon-chain artifact omits it (ADX is x86-only).
BLST_PORTABLE := -D__BLST_PORTABLE__

PGO_beacon-chain := -pgo=cmd/beacon-chain/pprof.beacon-chain.samples.cpu.pb.gz

# Build timestamp stamped into the dist binaries. Defaults to "now"; override it to make the
# build reproducible (identical bytes across machines/times):
#   SOURCE_DATE_EPOCH=1781515081 make dist
# Flatten to a simply-expanded value so both stamps below share one instant, then derive the
# RFC3339 form from it (GNU `date -d @`, with a BSD/macOS `date -r` fallback).
SOURCE_DATE_EPOCH ?= $(shell date -u +%s)
SOURCE_DATE_EPOCH := $(SOURCE_DATE_EPOCH)
BUILD_DATE_UNIX   := $(SOURCE_DATE_EPOCH)
BUILD_DATE        := $(shell date -u -d @$(SOURCE_DATE_EPOCH) +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -r $(SOURCE_DATE_EPOCH) +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS_STAMPED   := $(LDFLAGS) \
           -X $(VERSION_PKG).buildDate=$(BUILD_DATE) \
           -X $(VERSION_PKG).buildDateUnix=$(BUILD_DATE_UNIX)

# Which distributed binaries `make dist` builds: those named on the command line, else all.
DIST_BINS := $(or $(strip $(filter $(CROSS_BINARIES),$(MAKECMDGOALS))),$(CROSS_BINARIES))

# Positional goals for dist, and any that aren't a CROSS_BINARIES (a typo, or a host-only binary
# that dist can't cross-build). DIST_BIN_BAD makes `make dist xxx` error like build/gen/e2e.
DIST_BIN_BAD := $(filter-out $(CROSS_BINARIES),$(filter-out dist,$(MAKECMDGOALS)))

platform ?=
ifeq ($(strip $(platform)),)
DIST_PLAT_SEL := $(ALL_PLATFORMS)
else
DIST_PLAT_SEL := $(subst $(comma),$(space),$(platform))
endif

DIST_PLAT_BAD := $(filter-out $(ALL_PLATFORMS),$(DIST_PLAT_SEL))
DIST_DOCKER_ARCHES := $(sort $(foreach s,$(filter $(DOCKER_PLATFORMS),$(DIST_PLAT_SEL)),$(word 2,$(subst /,$(space),$(s)))))
DIST_BIN_PLATS := $(sort $(filter $(CROSS_PLATFORMS),$(DIST_PLAT_SEL)) $(foreach a,$(DIST_DOCKER_ARCHES),linux/$(a)))
DIST_TARGETS   := $(foreach s,$(DIST_BIN_PLATS),$(filter $(s)/%,$(CROSS_TARGETS)))

DIST_LDFLAGS := $(LDFLAGS_STAMPED) -s -w
BUILD_MODE   := release
BUILD_PGO    := $(PGO_beacon-chain)
BUILD_CROSS_ENV = GO="$(GO)" DIST="$(DIST)" GIT_TAG="$(GIT_TAG)" \
	CGO_CFLAGS_LINUX_ARM64="$(CGO_CFLAGS_LINUX_ARM64)" BLST_PORTABLE="$(BLST_PORTABLE)" \
	LDFLAGS="$(DIST_LDFLAGS)" TAGFLAG="$(TAGFLAG)" PGO_beacon_chain="$(BUILD_PGO)" \
	BUILD_MODE="$(BUILD_MODE)"

DIST_DOCKER      := $(strip $(DIST_DOCKER_ARCHES))
DIST_DOCKER_BINS := $(filter $(DOCKER_IMAGES),$(DIST_BINS))
DIST_DOCKER_ENV   = GO="$(GO)" DIST="$(DIST)" GIT_TAG="$(GIT_TAG)" \
	DOCKER_TAG="$(DOCKER_TAG)" DOCKER_REGISTRY="$(DOCKER_REGISTRY)" \
	DOCKER_BINARIES="$(strip $(DIST_DOCKER_BINS))" DOCKER_ARCHES="$(strip $(DIST_DOCKER_ARCHES))"

.PHONY: dist
dist:
	@$(if $(DIST_BIN_BAD),echo "❌ dist: unknown binary(ies): $(DIST_BIN_BAD)  (one of: $(CROSS_BINARIES))" >&2; exit 1;) \
	$(if $(DIST_PLAT_BAD),echo "❌ dist: unknown platform(s): $(DIST_PLAT_BAD)  (valid: $(ALL_PLATFORMS))" >&2; exit 1;) \
	$(BUILD_CROSS_ENV) CROSS_BINARIES="$(DIST_BINS)" CROSS_TARGETS="$(strip $(DIST_TARGETS))" $(GO) run ./build/crossdocker \
	$(if $(DIST_DOCKER),&& $(DIST_DOCKER_ENV) $(GO) run ./build/docker,)

# ---------------------------------------------------------------------------
# Pre-fetch external spec-test data
# ---------------------------------------------------------------------------
.PHONY: testdata
testdata: ## Pre-fetch all external spec-test data (tests fetch lazily otherwise)
	$(GO) run ./tools/cmd/fetch-testdata

# ---------------------------------------------------------------------------
# Help (default target)
# ---------------------------------------------------------------------------
.DEFAULT_GOAL := help
.PHONY: help
help: ## Show this help
	@echo ""
	@printf '\033[38;5;214m'
	@echo ' ███████████                                              '
	@echo '░░███░░░░░███                                             '
	@echo ' ░███    ░███ ████████  █████ ████  █████  █████████████  '
	@echo ' ░██████████ ░░███░░███░░███ ░███  ███░░  ░░███░░███░░███ '
	@echo ' ░███░░░░░░   ░███ ░░░  ░███ ░███ ░░█████  ░███ ░███ ░███ '
	@echo ' ░███         ░███      ░███ ░███  ░░░░███ ░███ ░███ ░███ '
	@echo ' █████        █████     ░░███████  ██████  █████░███ █████'
	@echo '░░░░░        ░░░░░       ░░░░░███ ░░░░░░  ░░░░░ ░░░ ░░░░░ '
	@echo '                         ███ ░███                         '
	@echo '                        ░░██████                          '
	@echo '                         ░░░░░░                           '
	@printf '\033[0m'
	@echo ""
	@echo "Commands:"
	@printf "  \033[36m%-48s\033[0m %s\n" "make run <bin> [-- <args>]"    "Run a binary"
	@printf "  \033[36m%-48s\033[0m %s\n" "make test [$(TEST_KINDS)] [mode=no-race|race]" "Run tests (default: $(MODE_DEFAULT_test))"
	@printf "  \033[36m%-48s\033[0m %s\n" "make build <bin> [-- <flags>]" "Build a binary"
	@printf "  \033[36m%-48s\033[0m %s\n" "make gen [$(GEN_KINDS)]"                "Create generated code"
	@printf "  \033[36m%-48s\033[0m %s\n" "make e2e [suite|scenario]"              "Run end-to-end tests (default: presubmit)"
	@printf "  \033[36m%-48s\033[0m %s\n" "make dist [<bin>...] [platform=<platform>]"  "Build official release binaries (docker/<arch> also packages an OCI image)"
	@printf "  \033[36m%-48s\033[0m %s\n" "make testdata"                          "Pre-fetch external spec-test data"
	@printf "  \033[36m%-48s\033[0m %s\n" "make help"                              "Show this help"
	@echo ""
	@printf "%-17s %s\n" "<bin>:" "$(BINARIES)"
	@printf "%-17s %s\n" "<platform>:" "$(ALL_PLATFORMS)  (give one, or a comma-separated list)"
	@echo ""
	@echo "E2E:"
	@printf "%-17s %s\n" "scenarios:" "$(E2E_SCENARIOS)"
	@echo "suites:"
	@$(foreach s,$(E2E_SUITES),printf -- "- %-15s %s\n" "$(s):" "$(E2E_SUITE_$(s))";)
	@echo ""
	@printf "%-17s %s\n" "note:" "after '--', pass '--flag value' (not '--flag=value')"

# ---------------------------------------------------------------------------
# Positional-argument catch-all
# ---------------------------------------------------------------------------
.PHONY: $(POSITIONAL)
$(POSITIONAL): ; @:
%:
	@:
