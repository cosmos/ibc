# SPDX-License-Identifier: Apache-2.0

help: ## List repository commands
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

E2E_FLAGS ?= -count=1
E2E_MODE ?= fast
LICENSE_EYE_VERSION ?= 0.8.0

E2E_DIR := e2e
HARNESS_DIR := $(E2E_DIR)/internal/harness
GEN_SOLIDITY_ABI_DIR := gen/go/solidity-abi
SCRIPTS_DIR := scripts
CONTRACT_BINDINGS := $(addprefix $(GEN_SOLIDITY_ABI_DIR)/,\
	accessmanager escrow testerc20 counter iftsendcallconstructor iftbatchtransfershim)

build-link: ## Build the Link binary
	$(MAKE) -C link build

install-link: ## Install the Link binary
	$(MAKE) -C link install

doctor-e2e: ## Check the runtime dependencies used by e2e tests
	@command -v go >/dev/null || { echo "missing go" >&2; exit 1; }
	@command -v docker >/dev/null || { echo "missing docker; Docker is required for e2e modes and matrix generation" >&2; exit 1; }
	@docker info >/dev/null || { echo "docker daemon is not reachable" >&2; exit 1; }

doctor-e2e-tools: ## Check the generation and lint tools used by repository e2e checks
	@command -v forge >/dev/null || { echo "missing forge; Forge is required to verify test-app artifacts" >&2; exit 1; }
	@command -v bun >/dev/null || { echo "missing bun; bun is required to install Solidity contract dependencies" >&2; exit 1; }
	@command -v abigen >/dev/null || { echo "missing abigen; abigen is required to generate typed contract bindings" >&2; exit 1; }
	@command -v jq >/dev/null || { echo "missing jq; jq is required to generate typed contract bindings" >&2; exit 1; }
	@command -v golangci-lint >/dev/null || { echo "missing golangci-lint; it is required for e2e checks" >&2; exit 1; }

test-harness: build-link ## Run harness tests, including Docker-backed integrations when available
	go -C $(E2E_DIR) test ./internal/... ./cmd/...

test-e2e: build-link ## Run e2e tests (E2E_MODE=... E2E_FLAGS=...)
	# -parallel caps concurrent Docker environments; the GOMAXPROCS default can overload a large machine.
	E2E_MODE=$(E2E_MODE) go -C $(E2E_DIR) test . -timeout 60m -parallel 4 $(E2E_FLAGS)

generate-e2e-matrix: ## Regenerate the E2E provider and topology matrix (requires Docker)
	go -C $(E2E_DIR) run ./cmd/e2e-matrix -write test-matrix.md

check-e2e-matrix: ## Check that the E2E provider and topology matrix is current (requires Docker)
	go -C $(E2E_DIR) run ./cmd/e2e-matrix -check test-matrix.md

lint: lint-link lint-e2e lint-gen ## Lint all Go modules

lint-fix: lint-fix-link lint-fix-e2e lint-fix-gen ## Lint all Go modules and fix errors

lint-link: ## Lint the Link module
	$(MAKE) -C link lint

lint-fix-link: ## Lint the Link module and fix errors
	$(MAKE) -C link lint-fix

lint-e2e: ## Lint the e2e module, harness included
	cd $(E2E_DIR) && golangci-lint run

lint-fix-e2e: ## Lint the e2e module, harness included, and fix errors
	cd $(E2E_DIR) && golangci-lint run --fix

lint-gen: ## Lint generated Solidity Go binding packages
	cd $(GEN_SOLIDITY_ABI_DIR) && golangci-lint run

lint-fix-gen: ## Lint generated Solidity Go binding packages and fix errors
	cd $(GEN_SOLIDITY_ABI_DIR) && golangci-lint run --fix

test-gen: ## Compile generated Solidity Go binding packages
	go -C $(GEN_SOLIDITY_ABI_DIR) test ./...

clean-e2e-dry-run: ## Preview e2e processes and Docker resources
	$(E2E_DIR)/scripts/clean.sh --dry-run

clean-e2e: ## Kill e2e processes and remove Docker resources
	$(E2E_DIR)/scripts/clean.sh

test-apps: ## Rebuild test-app artifacts and typed Go bindings (requires bun, forge, abigen, and jq)
	bun install --cwd $(HARNESS_DIR)/environment/solidityibc/contracts --frozen-lockfile
	forge build --root $(HARNESS_DIR)/environment/solidityibc/contracts
	$(SCRIPTS_DIR)/generate-contract-bindings.sh

check-test-apps: ## Fail if typed Go contract bindings are stale
	bun install --cwd $(HARNESS_DIR)/environment/solidityibc/contracts --frozen-lockfile
	forge build --force --root $(HARNESS_DIR)/environment/solidityibc/contracts
	$(SCRIPTS_DIR)/generate-contract-bindings.sh
	@status="$$(git status --porcelain --untracked-files=all -- $(CONTRACT_BINDINGS))"; \
		test -z "$$status" || { \
			echo "contract bindings are stale — run 'make test-apps' and commit the result" >&2; \
			echo "$$status" >&2; \
			exit 1; \
		}

check-license-headers: ## Check SPDX license headers
	go run github.com/apache/skywalking-eyes/cmd/license-eye@v$(LICENSE_EYE_VERSION) --config .licenserc.yaml header check

check-link: ## Run Link-local checks
	$(MAKE) -C link check

check-gen: lint-gen test-gen ## Run all generated Solidity binding checks

check-e2e: doctor-e2e doctor-e2e-tools test-harness lint-e2e check-test-apps test-e2e check-e2e-matrix ## Run all repository e2e checks

check: check-license-headers check-link check-gen check-e2e ## Run license, Link, generated binding, and repository e2e checks

.PHONY: help build-link doctor-e2e doctor-e2e-tools test-harness test-e2e generate-e2e-matrix check-e2e-matrix lint lint-fix lint-link lint-fix-link lint-e2e lint-fix-e2e lint-gen lint-fix-gen test-gen \
	clean-e2e-dry-run clean-e2e test-apps check-test-apps check-license-headers check-link check-gen check-e2e check
