SHELL := /bin/bash
.SHELLFLAGS := -o pipefail -c

BAZEL   ?= bazel
DIST    ?= dist
BIN_DIR ?= bin

BAZEL_FLAGS ?=

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

E2E_TARGET_presubmit      := //testing/endtoend:presubmit
E2E_TARGET_postsubmit     := //testing/endtoend:postsubmit
E2E_TARGET_scenario_tests := //testing/endtoend:scenario_tests
E2E_TARGET_minimal        := //testing/endtoend:go_default_test
E2E_TARGET_slasher        := //testing/endtoend:go_default_test
E2E_TARGET_slashing       := //testing/endtoend:go_default_test
E2E_TARGET_statediff      := //testing/endtoend:go_default_test
E2E_TARGET_builder        := //testing/endtoend:go_builder_test
E2E_TARGET_postmerge      := //testing/endtoend:go_minimal_postmerge_test
E2E_TARGET_mainnet        := //testing/endtoend:go_mainnet_test
E2E_TARGET_scenario       := //testing/endtoend:go_minimal_scenario_test
E2E_TARGET_web3signer     :=
E2E_TARGET_multiclient    :=

POSITIONAL := $(sort $(GEN_KINDS) $(TEST_KINDS) $(E2E_KINDS) $(BINARIES))

GIT_TAG := $(shell git describe --tags --abbrev=0 2>/dev/null || echo Unknown)

CROSS_BINARIES := beacon-chain validator client-stats prysmctl
CROSS_TARGETS := \
	linux/amd64/x86_64-linux-gnu.2.31 \
	linux/arm64/aarch64-linux-gnu.2.31 \
	darwin/amd64/x86_64-macos \
	darwin/arm64/aarch64-macos \
	windows/amd64/x86_64-windows-gnu

CROSS_PLATFORMS := $(foreach t,$(CROSS_TARGETS),$(word 1,$(subst /,$(space),$(t)))/$(word 2,$(subst /,$(space),$(t))))

# <os>/<arch> binary platform -> bazel `--config=` flags for `make dist`, mirroring
# Prysm's official release script. linux/amd64 builds natively on the host; every
# other target cross-compiles inside the `docker-sandbox` (the `*_docker` configs
# carry the osxcross/mingw/aarch64 toolchains). NOTE: the `*_docker` configs require
# a Linux x86_64 host with Docker running (see build/bazelrc/cross.bazelrc).
DIST_CFG_linux_amd64   := --config=linux_amd64 --define=blst_modern=false
DIST_CFG_linux_arm64   := --config=linux_arm64_docker
DIST_CFG_darwin_amd64  := --config=osx_amd64_docker
DIST_CFG_darwin_arm64  := --config=osx_arm64_docker
DIST_CFG_windows_amd64 := --config=windows_amd64_docker

DEB_PACKAGES   := beacon-chain validator
DOCKER_IMAGES  := beacon-chain validator prysmctl
DOCKER_REGISTRY ?= gcr.io/offchainlabs/prysm
DOCKER_TAG      ?= $(GIT_TAG)
LINUX_ARCHES   := $(foreach t,$(filter linux/%,$(CROSS_TARGETS)),$(word 2,$(subst /,$(space),$(t))))
DEB_PLATFORMS  := $(foreach a,$(LINUX_ARCHES),deb/$(a))
DOCKER_PLATFORMS := $(foreach a,$(LINUX_ARCHES),docker/$(a))
ALL_PLATFORMS  := $(CROSS_PLATFORMS) $(DEB_PLATFORMS) $(DOCKER_PLATFORMS)

# `mode` selects the race setting and only applies to `make test` (`make test mode=race`).
TEST_MODES        := no-race race
MODE_DEFAULT_test := no-race
mode              ?= $(MODE_DEFAULT_test)

ifeq ($(filter $(mode),$(TEST_MODES)),)
$(error invalid mode '$(mode)' — only 'make test' takes a mode, one of: $(TEST_MODES))
endif

# ---------------------------------------------------------------------------
# Run a single binary (sugar over `bazel run //cmd/<bin>`)
# ---------------------------------------------------------------------------
RUN_GOALS := $(filter-out run,$(MAKECMDGOALS))
RUN_BIN   := $(firstword $(RUN_GOALS))
RUN_ARGS  := $(wordlist 2,$(words $(RUN_GOALS)),$(RUN_GOALS))

.PHONY: run
run:
	@bin="$(RUN_BIN)"; \
	[ -n "$$bin" ] || { echo "❌ run: name a binary, e.g. 'make run beacon-chain -- --help' (one of: $(BINARIES))" >&2; exit 1; }; \
	case " $(BINARIES) " in *" $$bin "*) ;; *) echo "❌ run: '$$bin' is not a binary (one of: $(BINARIES))" >&2; exit 1;; esac; \
	exec $(BAZEL) run $(BAZEL_FLAGS) "//cmd/$$bin" -- $(RUN_ARGS)

# ---------------------------------------------------------------------------
# Test
# ---------------------------------------------------------------------------
TEST_SEL  := $(filter $(TEST_KINDS),$(MAKECMDGOALS))
RACE_FLAG := $(if $(filter race,$(mode)),--features=race,)

.PHONY: test
test:
	@kinds="$(or $(strip $(TEST_SEL)),mainnet minimal)"; \
	for k in $$kinds; do \
	  case $$k in \
	    mainnet) echo "==> $(BAZEL) test //... $(RACE_FLAG)"; $(BAZEL) test //... $(RACE_FLAG) || exit 1;; \
	    minimal) echo "==> $(BAZEL) test //... --config=minimal $(RACE_FLAG)"; $(BAZEL) test //... --config=minimal $(RACE_FLAG) || exit 1;; \
	  esac; \
	done

# ---------------------------------------------------------------------------
# Build a single binary (sugar over `bazel build //cmd/<bin>`)
# ---------------------------------------------------------------------------
BUILD_GOALS := $(filter-out build,$(MAKECMDGOALS))
BUILD_BIN   := $(firstword $(BUILD_GOALS))
BUILD_ARGS  := $(wordlist 2,$(words $(BUILD_GOALS)),$(BUILD_GOALS))

.PHONY: build
build:
	@bin="$(BUILD_BIN)"; \
	[ -n "$$bin" ] || { echo "❌ build: name a binary, e.g. 'make build beacon-chain' (one of: $(BINARIES))" >&2; exit 1; }; \
	case " $(BINARIES) " in *" $$bin "*) ;; *) echo "❌ build: '$$bin' is not a binary (one of: $(BINARIES))" >&2; exit 1;; esac; \
	mkdir -p $(BIN_DIR); \
	echo "→ $(BAZEL) build //cmd/$$bin -> $(BIN_DIR)/$$bin"; \
	$(BAZEL) build $(BAZEL_FLAGS) $(BUILD_ARGS) "//cmd/$$bin" || exit 1; \
	src=$$($(BAZEL) cquery $(BAZEL_FLAGS) $(BUILD_ARGS) --output=files "//cmd/$$bin" 2>/dev/null | tail -n1); \
	[ -n "$$src" ] || { echo "❌ build: could not resolve output of //cmd/$$bin" >&2; exit 1; }; \
	cp -f "$$src" "$(BIN_DIR)/$$bin"; \
	chmod +x "$(BIN_DIR)/$$bin"

# ---------------------------------------------------------------------------
# Code generation
# ---------------------------------------------------------------------------
GEN_GOALS := $(filter-out gen,$(MAKECMDGOALS))
GEN_BAD   := $(filter-out $(GEN_KINDS),$(GEN_GOALS))
GEN_SEL   := $(or $(filter $(MAKECMDGOALS),$(GEN_KINDS)),$(GEN_KINDS))

.PHONY: gen
gen:
	@$(if $(GEN_BAD),echo "❌ gen: unknown kind(s): $(GEN_BAD)  (one of: $(GEN_KINDS))" >&2; exit 1;) \
	for k in $(GEN_SEL); do \
	  case $$k in \
	    proto) echo "==> gen proto"; ./hack/update-go-pbs.sh || exit 1;; \
	    ssz)   echo "==> gen ssz";   ./hack/update-go-ssz.sh || exit 1;; \
	    mocks) echo "==> gen mocks"; ./hack/update-mockgen.sh || exit 1;; \
	  esac; \
	done

# ---------------------------------------------------------------------------
# Static analysis
# ---------------------------------------------------------------------------
make_GOALS := $(filter-out lint,$(MAKECMDGOALS))
LINT_BAD   := $(filter-out fix,$(LINT_GOALS))

.PHONY: lint
lint:
	@$(if $(LINT_BAD),echo "❌ lint: unknown arg(s): $(LINT_BAD)  (only 'fix' is accepted)" >&2; exit 1;) :
ifeq ($(filter fix,$(MAKECMDGOALS)),fix)
	$(BAZEL) run //:gazelle -- fix
	$(BAZEL) run //:goimports_fix
	$(BAZEL) build //cmd/...
else
	$(BAZEL) run //:gazelle -- fix --mode=diff
	$(BAZEL) run //:goimports
	$(BAZEL) build //cmd/...
endif

# ---------------------------------------------------------------------------
# End-to-end tests
# ---------------------------------------------------------------------------
E2E_GOALS    := $(filter-out e2e,$(MAKECMDGOALS))
E2E_BAD      := $(filter-out $(E2E_KINDS),$(E2E_GOALS))
E2E_SEL      := $(or $(filter $(E2E_KINDS),$(MAKECMDGOALS)),presubmit)
E2E_TARGETS  := $(sort $(foreach k,$(E2E_SEL),$(E2E_TARGET_$(k))))
E2E_NOTARGET := $(strip $(foreach k,$(E2E_SEL),$(if $(E2E_TARGET_$(k)),,$(k))))

.PHONY: e2e
e2e:
	@$(if $(E2E_BAD),echo "❌ e2e: unknown suite/scenario(s): $(E2E_BAD)  (one of: $(E2E_KINDS))" >&2; exit 1;) :
	@$(if $(E2E_NOTARGET),echo "❌ e2e: no individually addressable Bazel target for: $(E2E_NOTARGET)  (run a suite, or one of: minimal builder slasher slashing scenario postmerge statediff mainnet)" >&2; exit 1;) :
	$(BAZEL) test $(E2E_TARGETS) --test_output=streamed

# ---------------------------------------------------------------------------
# Distribution build (official release binaries / .deb / OCI images, via Bazel)
# ---------------------------------------------------------------------------
DIST_BINS    := $(or $(strip $(filter $(CROSS_BINARIES),$(MAKECMDGOALS))),$(CROSS_BINARIES))
DIST_BIN_BAD := $(filter-out $(CROSS_BINARIES),$(filter-out dist,$(MAKECMDGOALS)))

platform ?=
ifeq ($(strip $(platform)),)
DIST_PLAT_SEL := $(ALL_PLATFORMS)
else
DIST_PLAT_SEL := $(subst $(comma),$(space),$(platform))
endif

DIST_PLAT_BAD      := $(filter-out $(ALL_PLATFORMS),$(DIST_PLAT_SEL))
DIST_BIN_PLATS     := $(sort $(filter $(CROSS_PLATFORMS),$(DIST_PLAT_SEL)))
DIST_DEB_ARCHES    := $(sort $(foreach s,$(filter $(DEB_PLATFORMS),$(DIST_PLAT_SEL)),$(word 2,$(subst /,$(space),$(s)))))
DIST_DOCKER_ARCHES := $(sort $(foreach s,$(filter $(DOCKER_PLATFORMS),$(DIST_PLAT_SEL)),$(word 2,$(subst /,$(space),$(s)))))
DIST_DEB_BINS      := $(filter $(DEB_PACKAGES),$(DIST_BINS))
DIST_DOCKER_BINS   := $(filter $(DOCKER_IMAGES),$(DIST_BINS))

.PHONY: dist
dist:
	@$(if $(DIST_BIN_BAD),echo "❌ dist: unknown binary(ies): $(DIST_BIN_BAD)  (one of: $(CROSS_BINARIES))" >&2; exit 1;) :
	@$(if $(DIST_PLAT_BAD),echo "❌ dist: unknown platform(s): $(DIST_PLAT_BAD)  (valid: $(ALL_PLATFORMS))" >&2; exit 1;) :
	@mkdir -p "$(DIST)"
	@for plat in $(DIST_BIN_PLATS); do \
	  os=$${plat%%/*}; arch=$${plat##*/}; \
	  case $$plat in \
	    linux/amd64)   cfg="$(DIST_CFG_linux_amd64)";; \
	    linux/arm64)   cfg="$(DIST_CFG_linux_arm64)";; \
	    darwin/amd64)  cfg="$(DIST_CFG_darwin_amd64)";; \
	    darwin/arm64)  cfg="$(DIST_CFG_darwin_arm64)";; \
	    windows/amd64) cfg="$(DIST_CFG_windows_amd64)";; \
	    *) echo "❌ dist: no cross config for $$plat" >&2; exit 1;; \
	  esac; \
	  ext=; [ "$$os" = windows ] && ext=.exe; \
	  echo "→ $(BAZEL) build --config=release $$cfg $(DIST_BINS:%=//cmd/%)"; \
	  $(BAZEL) build --config=release $$cfg $(DIST_BINS:%=//cmd/%) || exit 1; \
	  for bin in $(DIST_BINS); do \
	    src=$$($(BAZEL) cquery --config=release $$cfg --output=files "//cmd/$$bin" 2>/dev/null | tail -n1); \
	    [ -n "$$src" ] || { echo "❌ dist: cannot resolve //cmd/$$bin for $$plat" >&2; exit 1; }; \
	    cp -f "$$src" "$(DIST)/$$bin-$(GIT_TAG)-$$os-$$arch$$ext"; \
	    echo "   -> $(DIST)/$$bin-$(GIT_TAG)-$$os-$$arch$$ext"; \
	  done; \
	  if [ "$$plat" = linux/amd64 ] && (echo " $(DIST_BINS) " | grep -q ' beacon-chain '); then \
	    modern="--config=release --config=linux_amd64 --define=blst_modern=true --copt=-D__ADX__"; \
	    echo "→ $(BAZEL) build $$modern //cmd/beacon-chain (modern amd64)"; \
	    $(BAZEL) build $$modern //cmd/beacon-chain || exit 1; \
	    src=$$($(BAZEL) cquery $$modern --output=files //cmd/beacon-chain 2>/dev/null | tail -n1); \
	    [ -n "$$src" ] || { echo "❌ dist: cannot resolve modern //cmd/beacon-chain" >&2; exit 1; }; \
	    cp -f "$$src" "$(DIST)/beacon-chain-$(GIT_TAG)-modern-linux-amd64"; \
	    echo "   -> $(DIST)/beacon-chain-$(GIT_TAG)-modern-linux-amd64"; \
	  fi; \
	done
	@for arch in $(DIST_DEB_ARCHES); do \
	  if [ "$$arch" != amd64 ]; then echo "⚠ dist: skipping deb/$$arch — Bazel .deb targets are amd64-only" >&2; continue; fi; \
	  for bin in $(DIST_DEB_BINS); do \
	    tgt=//$$bin/package:deb; \
	    echo "→ $(BAZEL) build --config=release $$tgt"; \
	    $(BAZEL) build --config=release "$$tgt" || exit 1; \
	    f=$$($(BAZEL) cquery --config=release --output=files "$$tgt" 2>/dev/null | grep '\.deb$$' | tail -n1); \
	    [ -n "$$f" ] || { echo "❌ dist: cannot resolve .deb for $$tgt" >&2; exit 1; }; \
	    cp -f "$$f" "$(DIST)/"; \
	    echo "   -> $(DIST)/$$(basename $$f)"; \
	  done; \
	done
	@for arch in $(DIST_DOCKER_ARCHES); do \
	  case $$arch in \
	    amd64) cfg="$(DIST_CFG_linux_amd64)";; \
	    arm64) cfg="$(DIST_CFG_linux_arm64)";; \
	    *) echo "❌ dist: no docker config for arch $$arch" >&2; exit 1;; \
	  esac; \
	  for bin in $(DIST_DOCKER_BINS); do \
	    tgt=//cmd/$$bin:oci_image_tarball; \
	    echo "→ $(BAZEL) build --config=release $$cfg $$tgt"; \
	    $(BAZEL) build --config=release $$cfg "$$tgt" || exit 1; \
	    f=$$($(BAZEL) cquery --config=release $$cfg --output=files "$$tgt" 2>/dev/null | tail -n1); \
	    [ -n "$$f" ] || { echo "❌ dist: cannot resolve OCI tarball for $$tgt" >&2; exit 1; }; \
	    cp -f "$$f" "$(DIST)/$$bin-$(GIT_TAG)-linux-$$arch.tar"; \
	    echo "   -> $(DIST)/$$bin-$(GIT_TAG)-linux-$$arch.tar"; \
	  done; \
	done

# ---------------------------------------------------------------------------
# Pre-fetch external spec-test data
# ---------------------------------------------------------------------------
# Bazel fetches external archives lazily; this materializes the consensus spec
# test data up front. Other archives (bls/eip3076/eip4881, lighthouse,
# web3signer) are still fetched on demand when their tests build.
.PHONY: testdata
testdata: ## Pre-fetch consensus spec-test data (Bazel fetches the rest lazily)
	$(BAZEL) build @consensus_spec_tests//:test_data
	@execroot="$$($(BAZEL) info execution_root 2>/dev/null)"; \
	rel="$$($(BAZEL) cquery --output=files @consensus_spec_tests//:test_data 2>/dev/null | head -n1)"; \
	if [ -n "$$execroot" ] && [ -n "$$rel" ]; then \
	  dir="$$(echo "$$rel" | sed -E 's#(external/[^/]+)/.*#\1#')"; \
	  echo "→ spec-test data extracted under:"; \
	  echo "   $$execroot/$$dir"; \
	else \
	  echo "→ spec-test data built (resolve paths with: $(BAZEL) cquery --output=files @consensus_spec_tests//:test_data)"; \
	fi

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
	@echo "Bazel-backed: every command below dispatches to '$(BAZEL)'."
	@echo ""
	@echo "Commands:"
	@printf "  \033[36m%-48s\033[0m %s\n" "make run <bin> [BAZEL_FLAGS=<flags>] [-- <args>]" "Run a binary - Bazel"
	@printf "  \033[36m%-48s\033[0m %s\n" "make test [$(TEST_KINDS)] [mode=no-race|race]" "Run unit tests (default: $(MODE_DEFAULT_test)) - Bazel"
	@printf "  \033[36m%-48s\033[0m %s\n" "make build <bin>" "Build a binary - Bazel"
	@printf "  \033[36m%-48s\033[0m %s\n" "make gen [$(GEN_KINDS)]" "Create generated code - Bazel"
	@printf "  \033[36m%-48s\033[0m %s\n" "make lint [fix]" "Run lint - Bazel"
	@printf "  \033[36m%-48s\033[0m %s\n" "make e2e [suite|scenario]" "Run end-to-end tests (default: presubmit) - Bazel"
	@printf "  \033[36m%-48s\033[0m %s\n" "make dist [<bin>...] [platform=<platform>]" "Build release binaries - Bazel"
	@printf "  \033[36m%-48s\033[0m %s\n" "make testdata" "Pre-fetch consensus spec-test data - Bazel"
	@printf "  \033[36m%-48s\033[0m %s\n" "make help" "Show this help"
	@echo ""
	@printf "%-17s %s\n" "<bin>:" "$(BINARIES)"
	@printf "%-17s %s\n" "<platform>:" "$(ALL_PLATFORMS)  (give one, or a comma-separated list)"
	@echo ""
	@echo "E2E:"
	@printf "%-17s %s\n" "scenarios:" "$(E2E_SCENARIOS)"
	@printf "%-17s %s\n" "note:" "web3signer/multiclient have no individual Bazel target (run a suite instead)"
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
