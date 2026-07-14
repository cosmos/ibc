help: ## List repository commands
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

E2E_PKGS ?= ./setup ./ibclink ./external
E2E_FLAGS ?= -count=1
E2E_LANE ?= anvil

E2E_DIR := e2e
HARNESS_DIR := $(E2E_DIR)/internal/harness
LINK_BIN_DIR := $(CURDIR)/link/bin
TEST_APP_DIR := $(E2E_DIR)/internal/testapp/contracts

build-link: ## Build the Link binary
	$(MAKE) -C link build

build-stub: ## Build the temporary e2e stub into link/bin/ibc-stub
	mkdir -p $(LINK_BIN_DIR)
	go -C $(E2E_DIR) build -o $(LINK_BIN_DIR)/ibc-stub ./stub/cmd/ibc

doctor-e2e: ## Check the runtime dependencies used by e2e tests
	@command -v go >/dev/null || { echo "missing go" >&2; exit 1; }
	@command -v docker >/dev/null || { echo "missing docker; Docker is required for e2e lanes" >&2; exit 1; }
	@docker info >/dev/null || { echo "docker daemon is not reachable" >&2; exit 1; }

doctor-e2e-tools: ## Check the generation and lint tools used by repository e2e checks
	@command -v forge >/dev/null || { echo "missing forge; Forge is required to verify test-app artifacts" >&2; exit 1; }
	@command -v bun >/dev/null || { echo "missing bun; bun is required to install Solidity contract dependencies" >&2; exit 1; }
	@command -v abigen >/dev/null || { echo "missing abigen; abigen is required to generate typed contract bindings" >&2; exit 1; }
	@command -v jq >/dev/null || { echo "missing jq; jq is required to generate typed contract bindings" >&2; exit 1; }
	@command -v golangci-lint >/dev/null || { echo "missing golangci-lint; it is required for e2e checks" >&2; exit 1; }

test-harness: build-link ## Run harness tests, including Docker-backed integrations when available
	go -C $(HARNESS_DIR) test ./...

test-unit: ## Run pure-Go e2e selection, helper, and stub tests; no chains
	go -C $(E2E_DIR) test ./e2etest ./internal/... ./stub/...

test-e2e: build-link build-stub ## Run e2e tests (E2E_PKGS=... E2E_FLAGS=... E2E_LANE=...)
	E2E_LANE=$(E2E_LANE) go -C $(E2E_DIR) test $(E2E_PKGS) $(E2E_FLAGS)

lint-e2e: ## Lint the e2e and harness modules
	cd $(E2E_DIR) && golangci-lint run
	cd $(HARNESS_DIR) && golangci-lint run

lint-fix-e2e: ## Lint the e2e and harness modules and fix errors
	cd $(E2E_DIR) && golangci-lint run --fix
	cd $(HARNESS_DIR) && golangci-lint run --fix

clean-e2e-dry-run: ## Preview e2e processes and Docker resources
	$(E2E_DIR)/scripts/clean.sh --dry-run

clean-e2e: ## Kill e2e processes and remove Docker resources
	$(E2E_DIR)/scripts/clean.sh

test-apps: ## Rebuild test-app artifacts and typed Go bindings (requires bun, forge, abigen, and jq)
	forge build --root $(TEST_APP_DIR)
	bun install --cwd $(HARNESS_DIR)/internal/solidityibc/contracts --frozen-lockfile
	forge build --root $(HARNESS_DIR)/internal/solidityibc/contracts
	$(E2E_DIR)/scripts/generate-contract-bindings.sh

check-test-apps: ## Fail if typed Go contract bindings are stale
	forge build --force --root $(TEST_APP_DIR)
	bun install --cwd $(HARNESS_DIR)/internal/solidityibc/contracts --frozen-lockfile
	forge build --force --root $(HARNESS_DIR)/internal/solidityibc/contracts
	$(E2E_DIR)/scripts/generate-contract-bindings.sh
	@git diff --exit-code -- $(TEST_APP_DIR)/bindings \
		$(HARNESS_DIR)/internal/solidityibc/accessmanager || { \
		echo "contract bindings are stale — run 'make test-apps' and commit the result" >&2; exit 1; }

check-link: ## Run Link-local checks
	$(MAKE) -C link check

check-e2e: doctor-e2e doctor-e2e-tools test-harness test-unit lint-e2e check-test-apps test-e2e ## Run all repository e2e checks

check: check-link check-e2e ## Run Link and repository e2e checks

.PHONY: help build-link build-stub doctor-e2e doctor-e2e-tools test-harness test-unit test-e2e lint-e2e lint-fix-e2e \
	clean-e2e-dry-run clean-e2e test-apps check-test-apps check-link check-e2e check
