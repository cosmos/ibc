# SPDX-License-Identifier: Apache-2.0

help: ## List of commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

lint: ## Lint specs
	markdownlint-cli2 "**.md" "#0[0-8]-*.md"

lint-fix: ## Lint specs and fix errors
	markdownlint-cli2 "**.md" "#0[0-8]-*.md" --fix

.PHONY: help lint lint-fix
