SHELL := /bin/bash
.SHELLFLAGS := -o pipefail -c

GO   ?= go
DIST ?= dist

BINARIES := $(notdir $(patsubst %/,%,$(dir $(wildcard cmd/*/main.go))))
GEN_KINDS := proto ssz mocks
TEST_KINDS := mainnet mainnet-spectest minimal minimal-spectest
POSITIONAL := $(sort $(GEN_KINDS) $(TEST_KINDS) $(BINARIES))
COMMANDS := run build gen clean help test testdata

TAGS ?=
TAGFLAG := $(if $(TAGS),-tags=$(TAGS),)

flags ?=

ALLOWED_VARS := GO DIST TAGS flags mode
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
	echo "→ $$cmd"; \
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
	  echo "→ $$cmd"; \
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
	@printf "  \033[36m%-44s\033[0m %s\n" "make testdata"                              "Pre-fetch external spec-test data"
	@printf "  \033[36m%-44s\033[0m %s\n" "make clean"                                 "Clean everything"
	@printf "  \033[36m%-44s\033[0m %s\n" "make help"                                  "Show this help"
	@echo ""
	@printf '\033[1mOptions:\033[0m\n'
	@printf "  \033[36m%-14s\033[0m %s\n" "<bin>:"       "$(BINARIES)"
	@printf "  \033[36m%-14s\033[0m %s\n" "gen <kind>:"  "$(GEN_KINDS)"
	@printf "  \033[36m%-14s\033[0m %s\n" "test <kind>:" "$(TEST_KINDS)"
	@echo ""
	@printf '\033[1mNotes:\033[0m\n'
	@echo "  After '--', pass '--flag value' (not '--flag=value')"
	@echo ""

# ---------------------------------------------------------------------------
# Positional-argument catch-all (lets `make gen proto` / `make build beacon-chain`
# name kinds and binaries as goals)
# ---------------------------------------------------------------------------
.PHONY: $(POSITIONAL)
$(POSITIONAL): ; @:
%:
	@:
