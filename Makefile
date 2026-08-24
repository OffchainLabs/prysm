SHELL := /bin/bash
.SHELLFLAGS := -o pipefail -c

GO   ?= go
DIST ?= dist

empty :=
space := $(empty) $(empty)
comma := ,

BINARIES := $(notdir $(patsubst %/,%,$(dir $(wildcard cmd/*/main.go))))
GEN_KINDS := proto ssz mocks
TEST_KINDS := mainnet mainnet-spectest minimal minimal-spectest

E2E_SCENARIOS := minimal builder web3signer slasher slashing scenario scenario-multiclient postmerge statediff mainnet multiclient
E2E_SUITES    := presubmit postsubmit scenario_tests
E2E_KINDS     := $(E2E_SCENARIOS) $(E2E_SUITES)

# Suite -> scenario(s) mapping (keep in sync with the `suites` map in build/e2e/main.go).
E2E_SUITE_presubmit      := minimal statediff slashing slasher
E2E_SUITE_postsubmit     := builder postmerge mainnet multiclient
E2E_SUITE_scenario_tests := scenario scenario-multiclient

POSITIONAL := $(sort $(GEN_KINDS) $(TEST_KINDS) $(E2E_KINDS) $(BINARIES))
COMMANDS := run build gen clean help test testdata e2e dist

TAGS ?=
TAGFLAG := $(if $(TAGS),-tags=$(TAGS),)

flags ?=

ALLOWED_VARS := GO DIST TAGS flags mode platform SOURCE_DATE_EPOCH
BAD_VARS := $(strip $(foreach v,$(.VARIABLES),$(if $(filter command line,$(origin $(v))),$(filter-out $(ALLOWED_VARS),$(v)))))
ifneq ($(BAD_VARS),)
$(error unknown variable(s): $(BAD_VARS)  (allowed: $(ALLOWED_VARS)))
endif

GEN_MODE     := $(or $(mode),no-force)
GEN_MODE_BAD := $(filter-out force no-force,$(GEN_MODE))

TEST_MODE     := $(or $(mode),no-race)
TEST_MODE_BAD := $(filter-out no-race race,$(TEST_MODE))
TEST_ARGS     := $(filter-out $(COMMANDS),$(MAKECMDGOALS))
TEST_BAD      := $(filter-out $(TEST_KINDS),$(TEST_ARGS))

E2E_ARGS := $(filter-out $(COMMANDS),$(MAKECMDGOALS))
E2E_BAD  := $(filter-out $(E2E_KINDS),$(E2E_ARGS))

# Goals left over after `run` and the binary name are the program's arguments.
# A leading `--` ends make's option parsing so `--flag value` reaches us as goals
# (caught as no-ops by the catch-all) rather than being treated as make options.
RUN_BIN  := $(filter $(BINARIES),$(MAKECMDGOALS))
RUN_ARGS := $(filter-out run $(COMMANDS) $(RUN_BIN),$(MAKECMDGOALS))

# ---------------------------------------------------------------------------
# Code generation
# ---------------------------------------------------------------------------
GEN_GOALS := $(filter-out gen,$(MAKECMDGOALS))
GEN_BAD   := $(filter-out $(GEN_KINDS),$(GEN_GOALS))

.PHONY: gen
gen:
	@$(if $(GEN_MODE_BAD),echo "❌ gen: invalid mode '$(GEN_MODE)'  (one of: force no-force)" >&2; exit 1;) \
	$(if $(GEN_BAD),echo "❌ gen: unknown kind(s): $(GEN_BAD)  (one of: $(GEN_KINDS))" >&2; exit 1;) \
	$(GO) run ./build/gen --mode=$(GEN_MODE) $(filter $(GEN_KINDS),$(MAKECMDGOALS))

.PHONY: clean
clean:
	rm -f .gen-cache.json
	rm -rf third_party/testdata
	$(GO) clean -cache -testcache -modcache -fuzzcache

# ---------------------------------------------------------------------------
# Run
# ---------------------------------------------------------------------------
.PHONY: run
run:
	@$(MAKE) --no-print-directory gen

	@bin="$(strip $(RUN_BIN))"; \
	case "$$bin" in \
	  "")    echo "❌ run: specify a binary (one of: $(BINARIES))" >&2; exit 1;; \
	  *" "*) echo "❌ run: only one binary at a time (got: $$bin)" >&2; exit 1;; \
	esac; \
	cmd="$(strip $(GO) run $(TAGFLAG) $(flags) ./cmd/$$bin $(RUN_ARGS))"; \
	echo "-> $$cmd"; \
	eval "$$cmd"

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------
.PHONY: build
build:
	@$(MAKE) --no-print-directory gen

	@bins="$(or $(strip $(filter $(BINARIES),$(MAKECMDGOALS))),$(BINARIES))"; \
	bad="$(strip $(filter-out $(COMMANDS) $(BINARIES),$(MAKECMDGOALS)))"; \
	[ -z "$$bad" ] || { echo "❌ build: not a binary: $$bad (available: $(BINARIES))" >&2; exit 1; }; \
	mkdir -p $(DIST); \
	for b in $$bins; do \
	  cmd="$(strip $(GO) build $(TAGFLAG) $(flags) -o \"$(DIST)/$$b\" ./cmd/$$b)"; \
	  echo "-> $$cmd"; \
	  eval "$$cmd" || exit 1; \
	done; \
	echo "✅ build ==> $(DIST)/"

# ---------------------------------------------------------------------------
# Test
# ---------------------------------------------------------------------------
.PHONY: test
test:
	@$(if $(TEST_MODE_BAD),echo "❌ test: invalid mode '$(TEST_MODE)'  (one of: no-race race)" >&2; exit 1;) \
	$(if $(TEST_BAD),echo "❌ test: unknown kind(s): $(TEST_BAD)  (one of: $(TEST_KINDS))" >&2; exit 1;) :

	@$(MAKE) --no-print-directory gen

	@GO="$(GO)" $(GO) run ./build/test $(if $(filter race,$(TEST_MODE)),-race,) $(TEST_ARGS)

.PHONY: testdata
testdata:
	$(GO) run ./tools/cmd/fetch-testdata

# ---------------------------------------------------------------------------
# End-to-end tests
# ---------------------------------------------------------------------------
.PHONY: e2e
e2e:
	@$(if $(E2E_BAD),echo "❌ e2e: unknown target(s): $(E2E_BAD)  (scenarios: $(E2E_SCENARIOS) - suites: $(E2E_SUITES))" >&2; exit 1;) \
	GO="$(GO)" DIST="$(DIST)" $(GO) run ./build/e2e $(filter $(E2E_KINDS),$(MAKECMDGOALS))

# ---------------------------------------------------------------------------
# Distribution build (official, all-platforms, in Docker)
# ---------------------------------------------------------------------------
CROSS_BINARIES := beacon-chain validator client-stats prysmctl
CROSS_TARGETS := \
	linux/amd64/x86_64-linux-gnu.2.31 \
	linux/arm64/aarch64-linux-gnu.2.31 \
	darwin/amd64/x86_64-macos \
	darwin/arm64/aarch64-macos \
	windows/amd64/x86_64-windows-gnu

CROSS_PLATFORMS := $(foreach t,$(CROSS_TARGETS),$(word 1,$(subst /,$(space),$(t)))/$(word 2,$(subst /,$(space),$(t))))

# Baseline C flags for the cgo deps (blst). `-O2` has to be spelled out: setting CGO_CFLAGS
# replaces the toolchain default ("-O2 -g") rather than adding to it.
CGO_CFLAGS_COMMON := -O2

# linux/arm64 C optimization flags.
CGO_CFLAGS_LINUX_ARM64 := -ftree-vectorize -funsafe-math-optimizations -fomit-frame-pointer

# blst (Prysm's CGO dep) defaults to ADX/modern on amd64. Force the portable path. The
# modern amd64 beacon-chain artifact omits it (ADX is x86-only).
BLST_PORTABLE := -D__BLST_PORTABLE__

PGO_beacon-chain := -pgo=cmd/beacon-chain/pprof.beacon-chain.samples.cpu.pb.gz

# Version stamps: only the dist binaries are stamped (host builds stay cache-friendly).
VERSION_PKG := github.com/OffchainLabs/prysm/v7/runtime/version
GIT_COMMIT  := $(shell git rev-parse HEAD 2>/dev/null)
GIT_TAG     := $(shell git describe --tags --abbrev=0 2>/dev/null || echo Unknown)

# Build timestamp stamped into the dist binaries. Defaults to "now". Override it to make the
# build reproducible (identical bytes across machines/times):
#   SOURCE_DATE_EPOCH=1781515081 make dist
# Flatten to a simply-expanded value so both stamps below share one instant, then derive the
# RFC3339 form from it (GNU `date -d @`, with a BSD/macOS `date -r` fallback).
SOURCE_DATE_EPOCH ?= $(shell date -u +%s)
SOURCE_DATE_EPOCH := $(SOURCE_DATE_EPOCH)
BUILD_DATE_UNIX   := $(SOURCE_DATE_EPOCH)
BUILD_DATE        := $(shell date -u -d @$(SOURCE_DATE_EPOCH) +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -r $(SOURCE_DATE_EPOCH) +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS_STAMPED := -X $(VERSION_PKG).gitCommit=$(GIT_COMMIT) \
           -X $(VERSION_PKG).gitTag=$(GIT_TAG) \
           -X $(VERSION_PKG).buildDate=$(BUILD_DATE) \
           -X $(VERSION_PKG).buildDateUnix=$(BUILD_DATE_UNIX)

# Positional goals for dist: the distributed binaries named on the command line, else all.
# DIST_BAD collects any goal that isn't a CROSS_BINARIES (a typo, or a host-only binary that
# dist can't cross-build), so `make dist xxx` errors like gen/test/e2e.
DIST_ARGS := $(filter-out $(COMMANDS),$(MAKECMDGOALS))
DIST_BAD  := $(filter-out $(CROSS_BINARIES),$(DIST_ARGS))
DIST_BINS := $(or $(strip $(filter $(CROSS_BINARIES),$(DIST_ARGS))),$(CROSS_BINARIES))

# Which run-targets `make dist` builds: empty platform= means all, otherwise platform= is one
# or a space/comma list of exact <os>/<arch> values from CROSS_PLATFORMS (each maps to its
# CROSS_TARGETS entry). DIST_PLAT_BAD collects any selector that isn't a known platform.
platform ?=
ifeq ($(strip $(platform)),)
DIST_TARGETS  := $(CROSS_TARGETS)
DIST_PLAT_BAD :=
else
DIST_PLAT_SEL := $(subst $(comma),$(space),$(platform))
DIST_PLAT_BAD := $(filter-out $(CROSS_PLATFORMS),$(DIST_PLAT_SEL))
DIST_TARGETS  := $(foreach s,$(filter $(CROSS_PLATFORMS),$(DIST_PLAT_SEL)),$(filter $(s)/%,$(CROSS_TARGETS)))
endif

# Env for the in-container dist build (build/crossdocker -> build/cross). dist is always a
# release build: stamped + stripped (-s -w) + PGO'd. crossbuild reads PGO_beacon_chain
# (underscore) and applies it only to beacon-chain.
DIST_LDFLAGS := $(LDFLAGS_STAMPED) -s -w
BUILD_CROSS_ENV = GO="$(GO)" DIST="$(DIST)" GIT_TAG="$(GIT_TAG)" \
	CGO_CFLAGS_COMMON="$(CGO_CFLAGS_COMMON)" CGO_CFLAGS_LINUX_ARM64="$(CGO_CFLAGS_LINUX_ARM64)" \
	BLST_PORTABLE="$(BLST_PORTABLE)" \
	LDFLAGS="$(DIST_LDFLAGS)" TAGFLAG="$(TAGFLAG)" PGO_beacon_chain="$(PGO_beacon-chain)" \
	BUILD_MODE="release"

.PHONY: dist
dist:
	@$(if $(DIST_BAD),echo "❌ dist: unknown binary(ies): $(DIST_BAD)  (one of: $(CROSS_BINARIES))" >&2; exit 1;) \
	$(if $(DIST_PLAT_BAD),echo "❌ dist: unknown platform(s): $(DIST_PLAT_BAD)  (valid: $(CROSS_PLATFORMS))" >&2; exit 1;) :

	@$(MAKE) --no-print-directory gen

	@$(BUILD_CROSS_ENV) CROSS_BINARIES="$(DIST_BINS)" CROSS_TARGETS="$(strip $(DIST_TARGETS))" $(GO) run ./build/crossdocker

# ---------------------------------------------------------------------------
# Help (default target)
# ---------------------------------------------------------------------------
.DEFAULT_GOAL := help
.PHONY: help
help: ## Show this help
	@echo ""
	@printf '\033[1;38;5;214m'
	@echo "Prysm - Ethereum consensus client"
	@printf '\033[0m'
	@echo ""
	@printf '\033[1mCommands:\033[0m\n'
	@printf "  \033[36m%-44s\033[0m %s\n" "make run <bin> [flags=...] [-- <args>]"     "Run a binary"
	@printf "  \033[36m%-44s\033[0m %s\n" "make build [<bin>...] [flags=...]"          "Build a binary (default: all)"
	@printf "  \033[36m%-44s\033[0m %s\n" "make gen [<kind>...] [mode=force|no-force]" "Create generated code (default: all,no-force)"
	@printf "  \033[36m%-44s\033[0m %s\n" "make test [<kind>...] [mode=no-race|race]"  "Run unit tests (default: all,no-race)"
	@printf "  \033[36m%-44s\033[0m %s\n" "make e2e [<scenario>|<suite>...]"           "Run end-to-end tests (default: presubmit)"
	@printf "  \033[36m%-44s\033[0m %s\n" "make dist [<bin>...] [platform=...]"        "Build official release binaries (default: all binaries,all platforms)"
	@printf "  \033[36m%-44s\033[0m %s\n" "make testdata"                              "Pre-fetch external spec-test data"
	@printf "  \033[36m%-44s\033[0m %s\n" "make clean"                                 "Clean everything"
	@printf "  \033[36m%-44s\033[0m %s\n" "make help"                                  "Show this help"
	@echo ""
	@printf '\033[1mOptions:\033[0m\n'
	@printf "  \033[36m%-16s\033[0m %s\n" "<bin>:"          "$(BINARIES)"
	@printf "  \033[36m%-16s\033[0m %s\n" "gen <kind>:"     "$(GEN_KINDS)"
	@printf "  \033[36m%-16s\033[0m %s\n" "test <kind>:"    "$(TEST_KINDS)"
	@printf "  \033[36m%-16s\033[0m %s\n" "e2e <scenario>:" "$(E2E_SCENARIOS)"
	@printf "  \033[36m%-16s\033[0m\n" "e2e <suite>:"
	@printf "    \033[36m%-16s\033[0m %s\n" "presubmit:"      "$(E2E_SUITE_presubmit)"
	@printf "    \033[36m%-16s\033[0m %s\n" "postsubmit:"     "$(E2E_SUITE_postsubmit)"
	@printf "    \033[36m%-16s\033[0m %s\n" "scenario_tests:" "$(E2E_SUITE_scenario_tests)"
	@printf "  \033[36m%-16s\033[0m %s\n" "dist <bin>:"     "$(CROSS_BINARIES)"
	@printf "  \033[36m%-16s\033[0m %s\n" "dist platform:"  "$(CROSS_PLATFORMS)  (give one, or a space/comma list)"
	@echo ""
	@printf '\033[1mNotes:\033[0m\n'
	@echo "  After '--', pass '--flag value' (not '--flag=value')"
	@echo "  'make dist SOURCE_DATE_EPOCH=<unix-seconds>' pins the build timestamp (reproducible build)"
	@echo ""

# ---------------------------------------------------------------------------
# Positional-argument catch-all (lets `make gen proto` / `make build beacon-chain`
# name kinds and binaries as goals)
# ---------------------------------------------------------------------------
.PHONY: $(POSITIONAL)
$(POSITIONAL): ; @:
%:
	@:
