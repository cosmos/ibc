# SPDX-License-Identifier: Apache-2.0

help: ## List harness commands
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

lint: ## Lint code
	golangci-lint run

lint-fix: ## Lint & fix code
	golangci-lint run --fix

test: _build-cli ## Run the harness tests
	go test ./... -count=1

# Hidden commands
_build-cli:
	$(MAKE) -C ../cli build build-test-bin

.PHONY: help lint lint-fix test _build-cli
