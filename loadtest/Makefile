# SPDX-License-Identifier: Apache-2.0

# Flags passed to the scenario, e.g. LOADTEST_FLAGS='--preset longrun --json'.
LOADTEST_FLAGS ?=

help: ## List load-test commands
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

build: ## Build the load-test binary into bin/loadtest
	go build -o bin/loadtest ./cmd/loadtest

lint: ## Lint code
	golangci-lint run

lint-fix: ## Lint & fix code
	golangci-lint run --fix

test: ## Run the unit tests (no Docker; the scenarios never run under go test)
	go test ./... -count=1

run-relayer: build _build-cli ## Run the relayer scenario (LOADTEST_FLAGS=...)
	./bin/loadtest relayer $(LOADTEST_FLAGS)

run-attestor: build _build-cli ## Run the attestor scenario (LOADTEST_FLAGS=...)
	./bin/loadtest attestor $(LOADTEST_FLAGS)

clean: ## Kill leftover harness processes and Docker resources after a hard crash
	$(MAKE) -C ../e2e clean

# Hidden commands
_build-cli:
	$(MAKE) -C ../cli build

.PHONY: help build lint lint-fix test run-relayer run-attestor clean _build-cli
