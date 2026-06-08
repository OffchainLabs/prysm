# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------
SHELL := /bin/bash
.SHELLFLAGS := -o pipefail -c

GO         ?= go
DIST       ?= dist
VERSION_PKG := github.com/OffchainLabs/prysm/v7/runtime/version

# Binaries: name -> main package. Extend as more are migrated off Bazel.
BINARIES := beacon-chain validator prysmctl bootnode
PKG_beacon-chain := ./cmd/beacon-chain
PKG_validator    := ./cmd/validator
PKG_prysmctl     := ./cmd/prysmctl
PKG_bootnode     := ./tools/bootnode

# Version stamping: replaces Bazel --stamp / hack/workspace_status.sh, setting the
# same runtime/version vars its x_defs did. gitTag uses --abbrev=0 to get a clean
# vX.Y.Z (it feeds version.SemanticVersion()/BuildData()), matching workspace_status.sh.
GIT_COMMIT      := $(shell git rev-parse HEAD 2>/dev/null)
GIT_TAG         := $(shell git describe --tags --abbrev=0 2>/dev/null || echo Unknown)
BUILD_DATE      := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
BUILD_DATE_UNIX := $(shell date -u +%s)
LDFLAGS := -X $(VERSION_PKG).gitCommit=$(GIT_COMMIT) \
           -X $(VERSION_PKG).gitTag=$(GIT_TAG) \
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
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Per-binary build targets (from BINARIES):"
	@for b in $(BINARIES); do \
		printf "  \033[36m%-18s\033[0m %s\n" "build-$$b" "Build only $$b"; \
	done

# ---------------------------------------------------------------------------
# Build & test
# ---------------------------------------------------------------------------
.PHONY: build
build: $(addprefix build-,$(BINARIES)) ## Build all binaries into $(DIST)/

.PHONY: $(addprefix build-,$(BINARIES))
$(addprefix build-,$(BINARIES)): build-%:
	@mkdir -p $(DIST)
	$(GO) build $(TAGFLAG) -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/$* $(PKG_$*)

# build-all builds every `package main` in the module (the 4 primaries plus all
# cmd/* and tools/* utilities) into $(DIST)/, discovered via `go list` so there is
# no list to maintain. Output names are the package basenames (all unique).
.PHONY: build-all
build-all: ## Build every main package (cmd/* + tools/*) into $(DIST)/
	@mkdir -p $(DIST)
	@for p in $$($(GO) list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./...); do \
		echo "building $$(basename $$p)"; \
		$(GO) build $(TAGFLAG) -trimpath -ldflags "$(LDFLAGS)" -o "$(DIST)/$$(basename $$p)" "$$p" || exit 1; \
	done

# release builds the shippable binaries the way Bazel's `--config=release` did:
# optimized (Go's default build), version-stamped, stripped (-s -w ≈ --strip=always),
# and PGO-optimized for beacon-chain (the only binary with a committed profile).
PGO_beacon-chain := -pgo=cmd/beacon-chain/pprof.beacon-chain.samples.cpu.pb.gz

.PHONY: release $(addprefix release-,$(BINARIES))
release: $(addprefix release-,$(BINARIES)) ## Build optimized, stripped, PGO'd release binaries
$(addprefix release-,$(BINARIES)): release-%:
	@mkdir -p $(DIST)
	$(GO) build $(TAGFLAG) -trimpath $(PGO_$*) -ldflags "$(LDFLAGS) -s -w" -o $(DIST)/$* $(PKG_$*)

.PHONY: testdata
testdata: ## Pre-fetch all external spec-test data (tests fetch lazily otherwise)
	$(GO) run ./tools/cmd/fetch-testdata

# Mainnet pass excludes E2E (Phase 8) and the minimal-config packages — the
# latter run in the separate minimal pass below.
TEST_EXCLUDE := /testing/endtoend|/testing/spectest/minimal|/beacon-chain/rpc/prysm/v1alpha1/beacon$$|/beacon-chain/rpc/prysm/v1alpha1/validator$$
TEST_PKGS = $$($(GO) list ./... | grep -vE '$(TEST_EXCLUDE)')

# Packages tested under -tags=minimal (minimal consensus config): the minimal spec
# tests, the beacon + validator rpc packages (eth_network=minimal in Bazel), and the
# minimal fieldparams test. fieldparams also runs in the mainnet pass (mainnet_test.go).
# validator is a minimal-config package (its TestMain sets minimal config), so it runs
# only here; its few mainnet-only tests are tagged //go:build !minimal to keep them out
# of the minimal build (they aren't run in either pass, as before Phase 3 — running them
# would need a dedicated mainnet harness).
MINIMAL_PKGS := ./testing/spectest/minimal/... ./beacon-chain/rpc/prysm/v1alpha1/beacon ./beacon-chain/rpc/prysm/v1alpha1/validator ./config/fieldparams
MINIMAL_TAGFLAG := -tags=$(TEST_TAGS),minimal

# gotestsum (pinned via the go.mod tool directive) wraps `go test`, reformats
# output, and reruns flaky failures up to 5 times — matching Bazel's
# --flaky_test_attempts=5. If more than RERUN_MAX distinct tests fail it's a real
# breakage, not flakiness, so reruns are skipped.
GOTESTSUM := $(GO) tool gotestsum
RERUN_ATTEMPTS ?= 5
RERUN_MAX ?= 1000
# --no-color=false forces color even though we pipe gotestsum into the progress
# counter (a pipe is not a TTY, so gotestsum would otherwise disable color).
# --hide-summary=skipped drops the per-skipped-test "=== SKIP:" list from the
# end-of-run summary (spec tests skip thousands of "unused type" cases); failures
# and errors are still summarized.
GOTESTSUM_FLAGS := --format=pkgname --no-color=false --hide-summary=skipped --rerun-fails=$(RERUN_ATTEMPTS) --rerun-fails-max-failures=$(RERUN_MAX)

# progress prepends a running [X/N] package counter to gotestsum's pkgname
# lines (those containing a ✓/✖/∅/↻ status icon — matched anywhere on the line
# since a leading ANSI color code now precedes the icon), so you can see roughly
# how many packages remain. $1 is the total package count.
define progress
awk -v t=$(1) 'BEGIN{w=length(t)} /(✓|✖|∅|↻)/{c++; printf "[%*d/%d] %s\n", w, c, t, $$0; fflush(); next} {print; fflush()}'
endef

.PHONY: test
test: ## Run unit tests: a mainnet pass then a minimal (-tags=minimal) pass.
	@set -o pipefail; \
	fail=0; \
	echo; echo "=== mainnet pass ==="; \
	total=$$( $(GO) list ./... | grep -vcE '$(TEST_EXCLUDE)' ); \
	$(GOTESTSUM) $(GOTESTSUM_FLAGS) --packages="$(TEST_PKGS)" -- $(TEST_TAGFLAG) | $(call progress,$$total) || fail=1; \
	echo; echo "=== minimal pass (-tags=minimal) ==="; \
	mtotal=$$( $(GO) list $(MINIMAL_PKGS) | wc -l | tr -d ' ' ); \
	$(GOTESTSUM) $(GOTESTSUM_FLAGS) --packages="$(MINIMAL_PKGS)" -- $(MINIMAL_TAGFLAG) | $(call progress,$$mtotal) || fail=1; \
	echo; \
	if [ $$fail -eq 0 ]; then echo "✅ All tests passed (mainnet + minimal; any 'failure' above was a flake recovered within $(RERUN_ATTEMPTS) attempts)"; \
	else echo "❌ Some failure: a test failed all $(RERUN_ATTEMPTS) attempts (mainnet or minimal pass)"; exit 1; fi

.PHONY: test-race
test-race: ## Run unit tests with the race detector
	@set -o pipefail; \
	echo; \
	total=$$( $(GO) list ./... | grep -vcE '$(TEST_EXCLUDE)' ); \
	$(GOTESTSUM) $(GOTESTSUM_FLAGS) --packages="$(TEST_PKGS)" -- $(TEST_TAGFLAG) -race | $(call progress,$$total) \
	  && { echo; echo "✅ All tests passed (any 'failure' above was a flake recovered within $(RERUN_ATTEMPTS) attempts)"; } \
	  || { echo; echo "❌ Some failure: At least one test failed all $(RERUN_ATTEMPTS) attempts"; exit 1; }

# ---------------------------------------------------------------------------
# Phase 1+ targets — not migrated off Bazel yet. Stubbed to fail loudly so that
# `make <target>` makes clear what still needs implementing, and `make help`
# lists the full surface. See BAZEL_MIGRATION.md.
# ---------------------------------------------------------------------------
.PHONY: gen gen-proto gen-ssz gen-mocks lint docker deb cross

gen: gen-proto gen-ssz gen-mocks ## Regenerate all generated code (proto, SSZ, mocks)

gen-proto: ## Regenerate *.pb.go via go.mod-pinned protoc-gen-go-cast
	@./hack/update-go-pbs.sh

gen-ssz: ## Regenerate *.ssz.go (mainnet) via go.mod-pinned sszgen
	@./hack/update-go-ssz.sh

gen-mocks: ## Regenerate gomock mocks (go.mod-pinned mockgen)
	@./hack/update-mockgen.sh

lint: ## [Phase 7] Static analysis (nogo → prysm-vet multichecker)
	@echo "❌ 'lint' is not implemented yet — Phase 7 (static analysis). See BAZEL_MIGRATION.md."; exit 1

cross: ## [Phase 4] Cross-compile binaries via zig cc
	@echo "❌ 'cross' is not implemented yet — Phase 4 (CGO cross-compilation). See BAZEL_MIGRATION.md."; exit 1

docker: ## [Phase 5] Build OCI/Docker images
	@echo "❌ 'docker' is not implemented yet — Phase 5 (Docker/OCI images). See BAZEL_MIGRATION.md."; exit 1

deb: ## [Phase 6] Build .deb packages
	@echo "❌ 'deb' is not implemented yet — Phase 6 (.deb packaging). See BAZEL_MIGRATION.md."; exit 1

.PHONY: clean
clean: ## Remove build output
	rm -rf $(DIST)

