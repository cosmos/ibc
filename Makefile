help: ## List repository commands
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

E2E_PKGS ?= ./setup ./ibclink ./external
E2E_FLAGS ?= -count=1
E2E_LANE ?= anvil

E2E_DIR := e2e
HARNESS_DIR := $(E2E_DIR)/internal/harness
LINK_BIN_DIR := $(CURDIR)/link/bin
TEST_APP_DIR := $(E2E_DIR)/internal/testapp/contracts
TEST_APP_ARTIFACTS := \
	$(TEST_APP_DIR)/out/Counter.sol/Counter.json \
	$(TEST_APP_DIR)/out/TestAppDeployer.sol/TestAppDeployer.json \
	$(TEST_APP_DIR)/out/MockGMP.sol/MockGMP.json \
	$(TEST_APP_DIR)/out/MockIFT.sol/MockIFT.json

build-link: ## Build the Link binary
	$(MAKE) -C link build

build-stub: $(TEST_APP_ARTIFACTS) ## Build the temporary e2e stub into link/bin/ibc-stub
	mkdir -p $(LINK_BIN_DIR)
	go -C $(E2E_DIR) build -o $(LINK_BIN_DIR)/ibc-stub ./stub/cmd/ibc

doctor-e2e: $(TEST_APP_ARTIFACTS) ## Check the e2e toolchain and configured SUT paths
	@command -v go >/dev/null || { echo "missing go" >&2; exit 1; }
	@command -v docker >/dev/null || { echo "missing docker; Docker is required for e2e lanes" >&2; exit 1; }
	@command -v forge >/dev/null || { echo "missing forge; Forge is required to verify test-app artifacts" >&2; exit 1; }
	@command -v golangci-lint >/dev/null || { echo "missing golangci-lint; it is required for e2e checks" >&2; exit 1; }
	@docker info >/dev/null || { echo "docker daemon is not reachable" >&2; exit 1; }
	@if [ -n "$$IBC_BIN" ]; then test -x "$$IBC_BIN" || { echo "IBC_BIN is not executable: $$IBC_BIN" >&2; exit 1; }; echo "IBC_BIN=$$IBC_BIN"; else echo "IBC_BIN=link/bin/ibc (built by build-link)"; fi
	@if [ -n "$$IBC_STUB_BIN" ]; then test -x "$$IBC_STUB_BIN" || { echo "IBC_STUB_BIN is not executable: $$IBC_STUB_BIN" >&2; exit 1; }; echo "IBC_STUB_BIN=$$IBC_STUB_BIN"; else echo "IBC_STUB_BIN=link/bin/ibc-stub (built by build-stub)"; fi

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

test-apps: ## Rebuild embedded test-application artifacts (requires forge)
	forge build --root $(TEST_APP_DIR)

check-test-apps: ## Fail if embedded test-application artifacts are stale
	forge build --force --root $(TEST_APP_DIR)
	@git diff --exit-code -- $(TEST_APP_DIR)/out || { \
		echo "$(TEST_APP_DIR)/out is stale — run 'make test-apps' and commit the result" >&2; exit 1; }

check-link: ## Run Link-local checks
	$(MAKE) -C link check

check-e2e: doctor-e2e test-harness test-unit lint-e2e check-test-apps test-e2e ## Run all repository e2e checks

check: check-link check-e2e ## Run Link and repository e2e checks

$(TEST_APP_ARTIFACTS):
	@echo "missing test-application artifact $@ — run 'make test-apps' (requires forge)" >&2; exit 1

.PHONY: help build-link build-stub doctor-e2e test-harness test-unit test-e2e lint-e2e lint-fix-e2e \
	clean-e2e-dry-run clean-e2e test-apps check-test-apps check-link check-e2e check
