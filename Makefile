GO ?= go

GEN_KINDS := proto ssz mocks

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
	@printf "  \033[36m%-28s\033[0m %s\n" "make gen [$(GEN_KINDS)]" "Create generated code (default: all)"
	@printf "  \033[36m%-28s\033[0m %s\n" "make help"               "Show this help"
	@echo ""

# ---------------------------------------------------------------------------
# Positional-argument catch-all (lets `make gen proto` name kinds as goals)
# ---------------------------------------------------------------------------
.PHONY: $(GEN_KINDS)
$(GEN_KINDS): ; @:
%:
	@:
