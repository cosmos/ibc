# SPDX-License-Identifier: Apache-2.0

# Flags passed to a load-test scenario, e.g. LOADTEST_FLAGS='--preset longrun --json'.
LOADTEST_FLAGS ?=

help: ## List harness commands
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

build: ## Build the load-test binary into bin/loadtest
	go build -o bin/loadtest ./cmd/loadtest

lint: ## Lint code
	golangci-lint run

lint-fix: ## Lint & fix code
	golangci-lint run --fix

test: _build-cli ## Run the harness and load-test unit tests
	go test ./... -count=1

run-relayer: build _build-cli ## Run the relayer load scenario (LOADTEST_FLAGS=...)
	./bin/loadtest relayer $(LOADTEST_FLAGS)

run-attestor: build _build-cli ## Run the attestor load scenario (LOADTEST_FLAGS=...)
	./bin/loadtest attestor $(LOADTEST_FLAGS)

clean: ## Kill leftover harness processes and Docker resources after a hard crash
	$(MAKE) -C ../e2e clean

run-all-checks: ## Run "all-in-one" code validation step.
	@echo "==== 1. Linting ===="
	$(MAKE) lint

	@echo "==== 2. Building ===="
	$(MAKE) build

	@echo "==== 3. Testing ===="
	$(MAKE) test

# Hidden commands
_build-cli:
	$(MAKE) -C ../cli build build-test-bin

.PHONY: help build lint lint-fix test run-relayer run-attestor clean run-all-checks _build-cli
