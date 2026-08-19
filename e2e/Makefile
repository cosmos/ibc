# SPDX-License-Identifier: Apache-2.0

E2E_FLAGS ?= -count=1
E2E_MODE ?= fast
FORGE_BUILD_FLAGS ?=

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

lint: ## Lint code
	golangci-lint run

lint-fix: ## Lint & fix code
	golangci-lint run --fix

test-harness: _build-link ## Run tests for e2e harness itself
	go test ./internal/... ./cmd/... -count=1

test-apps: ## Rebuild test-app artifacts and typed Go bindings (requires bun, forge, abigen, and jq)
	bun install --cwd $(HARNESS_DIR)/environment/solidityibc/contracts --frozen-lockfile
	forge build $(FORGE_BUILD_FLAGS) --root $(HARNESS_DIR)/environment/solidityibc/contracts
	./scripts/generate-contract-bindings.sh

test: _build-link ## Run e2e tests (E2E_MODE=... E2E_FLAGS=...)
	# -parallel caps concurrent Docker environments; the GOMAXPROCS default can overload a large machine.
	E2E_MODE=$(E2E_MODE) go test . -timeout 60m -parallel 4 $(E2E_FLAGS)

generate-matrix: ## Regenerate the E2E provider and topology matrix
	go run ./cmd/e2e-matrix -write test-matrix.md

check-matrix: ## Check that the generated E2E matrix is current
	go run ./cmd/e2e-matrix -check test-matrix.md

clean: ## Kill e2e processes and remove Docker resources
	./scripts/clean.sh

clean-dry-run: ## Preview e2e processes and Docker resources
	./scripts/clean.sh --dry-run

check-stale: ## Run all checks for stale code
	@echo "Checking for stale ABI Go bindings..."
	$(MAKE) test-apps FORGE_BUILD_FLAGS=--force
	@status="$$(git status --porcelain --untracked-files=all -- $(CONTRACT_BINDINGS))"; \
		test -z "$$status" || { \
			echo "contract bindings are stale — run 'make -C e2e test-apps' and commit the result" >&2; \
			echo "$$status" >&2; \
			exit 1; \
		}

run-all-checks: doctor doctor-tools ## Run "all-in-one" code validation step.
	@echo "==== 1. Linting ===="
	$(MAKE) lint

	@echo "==== 2. Checking for stale code ===="
	$(MAKE) check-stale

	@echo "==== 3. Testing harness ===="
	$(MAKE) test-harness

	@echo "==== 4. Running E2E ===="
	$(MAKE) test

	@echo "==== 5. Checking matrix ===="
	$(MAKE) check-matrix

# Hidden commands
_build-link:
	$(MAKE) -C ../link build

.PHONY: help doctor doctor-tools lint lint-fix
.PHONY: test-harness test-apps test
.PHONY: generate-matrix check-matrix
.PHONY:	clean clean-dry-run check-stale run-all-checks
.PHONY: _build-link
