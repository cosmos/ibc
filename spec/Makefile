MARKDOWNLINT_CLI2 ?= npx --yes markdownlint-cli2@0.22.1

help: ## List of commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

lint: ## Lint specs
	$(MARKDOWNLINT_CLI2) "**.md"

lint-fix: ## Lint specs and fix errors
	$(MARKDOWNLINT_CLI2) "**.md" --fix

.PHONY: help lint lint-fix
