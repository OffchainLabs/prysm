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
PKG_validator    := ./cmd/validatorI 
PKG_prysmctl     := ./cmd/prysmctl
PKG_bootnode     := ./tools/bootnode

# Version stamping (replaces Bazel --stamp / workspace_status.sh; fleshed out in Phase 2).
GIT_COMMIT      := $(shell git rev-parse HEAD 2>/dev/null)
GIT_TAG         := $(shell git describe --tags --always 2>/dev/null)
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

.PHONY: testdata
testdata: ## Download external spec-test data
	@./hack/testdata.sh

# TODO: HANDLE THAT:
# Exclude all tests needing minimal configs + E2E (defer to BAZEL_MIGRATION.md Phase 3).
TEST_EXCLUDE := /testing/endtoend|/testing/spectest/minimal|/beacon-chain/rpc/prysm/v1alpha1/beacon$$|/beacon-chain/rpc/prysm/v1alpha1/validator$$
TEST_PKGS = $$($(GO) list ./... | grep -vE '$(TEST_EXCLUDE)')

# gotestsum (pinned via the go.mod tool directive) wraps `go test`, reformats
# output, and reruns flaky failures up to 5 times — matching Bazel's
# --flaky_test_attempts=5. If more than RERUN_MAX distinct tests fail it's a real
# breakage, not flakiness, so reruns are skipped.
GOTESTSUM := $(GO) tool gotestsum
RERUN_ATTEMPTS ?= 5
RERUN_MAX ?= 1000
# --no-color=false forces color even though we pipe gotestsum into the progress
# counter (a pipe is not a TTY, so gotestsum would otherwise disable color).
GOTESTSUM_FLAGS := --format=pkgname --no-color=false --rerun-fails=$(RERUN_ATTEMPTS) --rerun-fails-max-failures=$(RERUN_MAX)

# progress prepends a running [X/N] package counter to gotestsum's pkgname
# lines (those containing a ✓/✖/∅/↻ status icon — matched anywhere on the line
# since a leading ANSI color code now precedes the icon), so you can see roughly
# how many packages remain. $1 is the total package count.
define progress
awk -v t=$(1) 'BEGIN{w=length(t)} /(✓|✖|∅|↻)/{c++; printf "[%*d/%d] %s\n", w, c, t, $$0; fflush(); next} {print; fflush()}'
endef

.PHONY: test
test: testdata ## Run unit tests (mainnet config). Use `make test TAGS=minimal` for minimal.
	@set -o pipefail; \
	echo; \
	total=$$( $(GO) list ./... | grep -vcE '$(TEST_EXCLUDE)' ); \
	$(GOTESTSUM) $(GOTESTSUM_FLAGS) --packages="$(TEST_PKGS)" -- $(TEST_TAGFLAG) | $(call progress,$$total) \
	  && { echo; echo "✅ All tests passed (any 'failure' above was a flake recovered within $(RERUN_ATTEMPTS) attempts)"; } \
	  || { echo; echo "❌ Some failure: a test failed all $(RERUN_ATTEMPTS) attempts"; exit 1; }

.PHONY: test-race
test-race: testdata ## Run unit tests with the race detector
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

gen:       ## [Phase 1] Regenerate all generated code (proto, SSZ, mocks)
gen-proto: ## [Phase 1] Regenerate *.pb.go from .proto definitions
gen-ssz:   ## [Phase 1] Regenerate *.ssz.go (mainnet + minimal variants)
gen gen-proto gen-ssz:
	@echo "❌ '$@' is not implemented yet — Phase 1 (code generation). See BAZEL_MIGRATION.md."; exit 1

gen-mocks: ## [Phase 1] Regenerate gomock mocks (go.mod-pinned mockgen)
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

