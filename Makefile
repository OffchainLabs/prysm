GO ?= go

GEN_KINDS := proto ssz mocks

# ---------------------------------------------------------------------------
# Code generation
# ---------------------------------------------------------------------------
GEN_GOALS := $(filter-out gen,$(MAKECMDGOALS))
GEN_BAD   := $(filter-out $(GEN_KINDS),$(GEN_GOALS))

mode ?= no-force
GEN_MODE_BAD := $(filter-out force no-force,$(mode))

.PHONY: gen
gen:
	@$(if $(GEN_MODE_BAD),echo "❌ gen: invalid mode '$(mode)'  (one of: force no-force)" >&2; exit 1;) \
	$(if $(GEN_BAD),echo "❌ gen: unknown kind(s): $(GEN_BAD)  (one of: $(GEN_KINDS))" >&2; exit 1;) \
	$(GO) run ./build/gen --mode=$(mode) $(filter $(GEN_KINDS),$(MAKECMDGOALS))

.PHONY: clean
clean: 
	rm -f .gen-cache.json

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
	@printf "  \033[36m%-50s\033[0m %s\n" "make gen [$(GEN_KINDS)] [mode=force|no-force]" "Create generated code (default: all,no-force)"
	@printf "  \033[36m%-50s\033[0m %s\n" "make clean"                                    "Clean everything"
	@printf "  \033[36m%-50s\033[0m %s\n" "make help"                                     "Show this help"
	@echo ""

# ---------------------------------------------------------------------------
# Positional-argument catch-all (lets `make gen proto` name kinds as goals)
# ---------------------------------------------------------------------------
.PHONY: $(GEN_KINDS)
$(GEN_KINDS): ; @:
%:
	@:
