SHELL := /bin/bash
.SHELLFLAGS := -o pipefail -c

GO   ?= go
DIST ?= dist

BINARIES := $(notdir $(patsubst %/,%,$(dir $(wildcard cmd/*/main.go))))
GEN_KINDS := proto ssz mocks
TEST_KINDS := mainnet minimal

E2E_SCENARIOS := minimal builder web3signer slasher slashing scenario postmerge statediff mainnet multiclient
E2E_SUITES    := presubmit postsubmit scenario_tests
E2E_KINDS     := $(E2E_SCENARIOS) $(E2E_SUITES)

POSITIONAL := $(sort $(GEN_KINDS) $(TEST_KINDS) $(E2E_KINDS) $(BINARIES))

TAGS ?=
TAGFLAG := $(if $(TAGS),-tags=$(TAGS),)

empty :=
space := $(empty) $(empty)

VERSION_PKG     := github.com/OffchainLabs/prysm/v7/runtime/version
GIT_COMMIT      := $(shell git rev-parse HEAD 2>/dev/null)
GIT_TAG         := $(shell git describe --tags --abbrev=0 2>/dev/null || echo Unknown)
BUILD_DATE      := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
BUILD_DATE_UNIX := $(shell date -u +%s)

LDFLAGS := -X $(VERSION_PKG).gitCommit=$(GIT_COMMIT) \
           -X $(VERSION_PKG).gitTag=$(GIT_TAG)

LDFLAGS_STAMPED := $(LDFLAGS) \
           -X $(VERSION_PKG).buildDate=$(BUILD_DATE) \
           -X $(VERSION_PKG).buildDateUnix=$(BUILD_DATE_UNIX)

PGO_beacon-chain := -pgo=cmd/beacon-chain/pprof.beacon-chain.samples.cpu.pb.gz

MODE_VERBS         := test build

MODES_test         := no-race race
MODE_DEFAULT_test  := no-race

MODES_build        := dev release
MODE_DEFAULT_build := dev

MODE_VERB   := $(or $(firstword $(filter $(MODE_VERBS),$(MAKECMDGOALS))),build)
VALID_MODES := $(MODES_$(MODE_VERB))
mode        ?= $(MODE_DEFAULT_$(MODE_VERB))

ifeq ($(filter $(mode),$(VALID_MODES)),)
$(error invalid mode '$(mode)' — for this target use one of: $(VALID_MODES))
endif

RELEASE       := $(filter release,$(mode))
BUILD_LDFLAGS := $(if $(RELEASE),$(LDFLAGS_STAMPED) -s -w,$(LDFLAGS))

# ---------------------------------------------------------------------------
# Help (default target)
# ---------------------------------------------------------------------------
.DEFAULT_GOAL := help
.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z0-9_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Run a subset by naming the part(s); omit to do all (e.g. 'make build beacon-chain', 'make gen proto'):"
	@printf "  \033[36m%-14s\033[0m %s\n" "build" "$(BINARIES) [mode=$(subst $(space),|,$(MODES_build))] (default: $(MODE_DEFAULT_build))"
	@printf "  \033[36m%-14s\033[0m %s\n" "gen" "$(GEN_KINDS)"
	@printf "  \033[36m%-14s\033[0m %s\n" "test" "$(TEST_KINDS) [mode=$(subst $(space),|,$(MODES_test))] (default: $(MODE_DEFAULT_test))"
	@printf "  \033[36m%-14s\033[0m %s\n" "e2e" "scenarios: $(E2E_SCENARIOS)"
	@printf "  \033[36m%-14s\033[0m %s\n" ""    "suites:    $(E2E_SUITES) (default: presubmit)"

# ---------------------------------------------------------------------------
# Code generation
# ---------------------------------------------------------------------------
.PHONY: gen
gen:
	@$(GO) run ./build/gen $(filter $(GEN_KINDS),$(MAKECMDGOALS))

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------
.PHONY: build
build:
	@bins="$(or $(strip $(filter $(BINARIES),$(MAKECMDGOALS))),$(BINARIES))"; \
	bad="$(strip $(filter-out $(BINARIES),$(filter $(POSITIONAL),$(MAKECMDGOALS))))"; \
	[ -z "$$bad" ] || { echo "❌ build: not a binary: $$bad (available: $(BINARIES))" >&2; exit 1; }; \
	mkdir -p $(DIST); \
	for b in $$bins; do \
	  pgo=""; $(if $(RELEASE),[ "$$b" = beacon-chain ] && pgo="$(PGO_beacon-chain)";) \
	  echo "→ $(mode)  $$b"; \
	  $(GO) build $(TAGFLAG) -trimpath $$pgo -ldflags "$(BUILD_LDFLAGS)" -o "$(DIST)/$$b" "./cmd/$$b" || exit 1; \
	done; \
	echo "✅ build ==> $(DIST)/"

# ---------------------------------------------------------------------------
# Test
# ---------------------------------------------------------------------------
# The package sets, build tags, gotestsum flags and flaky-rerun budget all live in
# build/test (the single source of truth); the Makefile just dispatches to it.
.PHONY: test
test:
	@GO="$(GO)" $(GO) run ./build/test $(if $(filter race,$(mode)),-race,) $(filter $(POSITIONAL),$(MAKECMDGOALS))

.PHONY: testdata
testdata: ## Pre-fetch all external spec-test data (tests fetch lazily otherwise)
	$(GO) run ./tools/cmd/fetch-testdata

# ---------------------------------------------------------------------------
# End-to-end tests
# ---------------------------------------------------------------------------
.PHONY: e2e
e2e:
	@GO="$(GO)" DIST="$(DIST)" $(GO) run ./build/e2e $(filter $(E2E_KINDS),$(MAKECMDGOALS))
