# SPDX-License-Identifier: Apache-2.0

E2E_FLAGS ?= -count=1
E2E_MODE ?= fast

HARNESS_DIR := internal/harness
CONTRACT_BINDINGS := $(addprefix $(HARNESS_DIR)/environment/solidityibc/,\
	accessmanager escrow testerc20 counter iftsendcallconstructor iftbatchtransfershim)

help: ## List e2e commands
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

doctor: ## Check the runtime dependencies used by e2e tests
	@command -v go >/dev/null || { echo "missing go" >&2; exit 1; }
	@command -v docker >/dev/null || { echo "missing docker; Docker is required for e2e modes and matrix generation" >&2; exit 1; }
	@docker info >/dev/null || { echo "docker daemon is not reachable" >&2; exit 1; }

doctor-tools: ## Check the generation and lint tools used by e2e checks
	@command -v forge >/dev/null || { echo "missing forge; Forge is required to verify test-app artifacts" >&2; exit 1; }
	@command -v bun >/dev/null || { echo "missing bun; bun is required to install Solidity contract dependencies" >&2; exit 1; }
	@command -v abigen >/dev/null || { echo "missing abigen; abigen is required to generate typed contract bindings" >&2; exit 1; }
	@command -v jq >/dev/null || { echo "missing jq; jq is required to generate typed contract bindings" >&2; exit 1; }
	@command -v golangci-lint >/dev/null || { echo "missing golangci-lint; it is required for e2e checks" >&2; exit 1; }

test-harness: _build-link ## Run tests for e2e harness itself
	go test ./internal/... ./cmd/... -count=1

test-apps: ## Rebuild test-app artifacts and typed Go bindings (requires bun, forge, abigen, and jq)
	bun install --cwd $(HARNESS_DIR)/environment/solidityibc/contracts --frozen-lockfile
	forge build --root $(HARNESS_DIR)/environment/solidityibc/contracts
	./scripts/generate-contract-bindings.sh

test: _build-link ## Run e2e tests (E2E_MODE=... E2E_FLAGS=...)
	# -parallel caps concurrent Docker environments; the GOMAXPROCS default can overload a large machine.
	E2E_MODE=$(E2E_MODE) go test . -timeout 60m -parallel 4 $(E2E_FLAGS)

generate-matrix: ## Regenerate the E2E provider and topology matrix
	go run ./cmd/e2e-matrix -write test-matrix.md

check-matrix: ## Check that the generated E2E matrix is current
	go run ./cmd/e2e-matrix -check test-matrix.md

lint: ## Lint code
	golangci-lint run

lint-fix: ## Lint & fix code
	golangci-lint run --fix

clean-dry-run: ## Preview e2e processes and Docker resources
	./scripts/clean.sh --dry-run

clean: ## Kill e2e processes and remove Docker resources
	./scripts/clean.sh

check-test-apps: ## Fail if typed Go contract bindings are stale
	bun install --cwd $(HARNESS_DIR)/environment/solidityibc/contracts --frozen-lockfile
	forge build --force --root $(HARNESS_DIR)/environment/solidityibc/contracts
	./scripts/generate-contract-bindings.sh
	@status="$$(git -C .. status --porcelain --untracked-files=all -- $(addprefix e2e/,$(CONTRACT_BINDINGS)))"; \
		test -z "$$status" || { \
			echo "contract bindings are stale — run 'make test-apps' and commit the result" >&2; \
			echo "$$status" >&2; \
			exit 1; \
		}

check: doctor doctor-tools test-harness lint check-test-apps test check-matrix ## Run all e2e checks

# Hidden commands
_build-link:
	$(MAKE) -C ../link build

.PHONY: help doctor doctor-tools lint lint-fix
.PHONY: test-harness test-apps test
.PHONY: generate-matrix check-matrix
.PHONY:	clean-dry-run clean check-test-apps check
