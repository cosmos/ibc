help: ## List of commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

lint: ## Lint specs
	markdownlint-cli2 "**.md"

lint-fix: ## Lint specs and fix errors
	markdownlint-cli2 "**.md" --fix

.PHONY: help lint lint-fix