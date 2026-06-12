SHELL := /bin/bash
.SHELLFLAGS := -o pipefail -c

GO   ?= go
DIST ?= dist

BINARIES := $(notdir $(patsubst %/,%,$(dir $(wildcard cmd/*/main.go))))
GEN_KINDS := proto ssz mocks
POSITIONAL := $(sort $(GEN_KINDS) $(BINARIES))
COMMANDS := build gen clean help

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
	$(GO) clean -cache -testcache -modcache -fuzzcache

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
	@printf "  \033[36m%-50s\033[0m %s\n" "make build [<bin>...] [flags=...]"             "Build a binary (default: all)"
	@printf "  \033[36m%-50s\033[0m %s\n" "make gen [$(GEN_KINDS)] [mode=force|no-force]" "Create generated code (default: all,no-force)"
	@printf "  \033[36m%-50s\033[0m %s\n" "make clean"                                    "Clean everything"
	@printf "  \033[36m%-50s\033[0m %s\n" "make help"                                     "Show this help"
	@echo ""
	@printf '\033[1mOptions:\033[0m\n'
	@printf "  \033[36m%-16s\033[0m %s\n" "<bin>:" "$(BINARIES)"
	@echo ""

# ---------------------------------------------------------------------------
# Positional-argument catch-all (lets `make gen proto` / `make build beacon-chain`
# name kinds and binaries as goals)
# ---------------------------------------------------------------------------
.PHONY: $(POSITIONAL)
$(POSITIONAL): ; @:
%:
	@:
