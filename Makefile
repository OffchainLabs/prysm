SHELL := /bin/bash
.SHELLFLAGS := -o pipefail -c

GO   ?= go
DIST ?= dist

BINARIES := $(notdir $(patsubst %/,%,$(dir $(wildcard cmd/*/main.go))))
GEN_KINDS := proto ssz mocks
POSITIONAL := $(sort $(GEN_KINDS) $(BINARIES))

TAGS ?=
TAGFLAG := $(if $(TAGS),-tags=$(TAGS),)

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

mode ?= dev
ifeq ($(filter $(mode),dev release),)
$(error invalid mode '$(mode)' — use mode=dev|release)
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
	@printf "  \033[36m%-14s\033[0m %s\n" "build" "$(BINARIES) [mode=dev|release] (default: dev)"
	@printf "  \033[36m%-14s\033[0m %s\n" "gen" "$(GEN_KINDS)"

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
