SHELL := /bin/bash
.SHELLFLAGS := -o pipefail -c

GO ?= go

GEN_KINDS := proto ssz mocks

POSITIONAL := $(GEN_KINDS)

# ---------------------------------------------------------------------------
# Help (default target)
# ---------------------------------------------------------------------------
.DEFAULT_GOAL := help
.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z0-9_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Run a subset by naming the part(s); omit to do all (e.g. 'make gen proto'):"
	@printf "  \033[36m%-14s\033[0m %s\n" "gen" "$(GEN_KINDS)"

# ---------------------------------------------------------------------------
# Code generation
# ---------------------------------------------------------------------------

.PHONY: gen
gen:
	@$(GO) run ./build/gen $(filter $(GEN_KINDS),$(MAKECMDGOALS))
