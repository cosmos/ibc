help: ## List of commands
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary
	go build -o bin/ibc ./cmd/ibc/...

lint: ## Lint the code
	golangci-lint run

lint-fix: ## Lint the code and fix errors
	golangci-lint run --fix

codegen: codegen-sql codegen-proto codegen-mocks ## Generate code

codegen-sql: ## Generate Go code based on migrations and queries
	cd internal/store && sqlc generate

codegen-proto: ## Generate Go proto bindings
	cd ../proto && buf generate

codegen-mocks: ## Generate mocks for the code
	go run github.com/vektra/mockery/v3@v3.7.1

test: ## Run tests
	go test ./... -v

# --- e2e ---------------------------------------------------------------------
# Black-box e2e: harness/ and e2e/ are separate Go modules; the SUT is the `ibc`
# binary driven only through its wire surface. Until real functionality lands,
# the stub SUT (e2e/stub) fills the gaps: it builds as bin/ibc-stub, the real
# binary as bin/ibc (`make build`). See e2e/README.md.

E2E_PKGS ?= ./setup ./ibclink ./external
E2E_FLAGS ?= -count=1
E2E_LANE ?= anvil

FIXTURE_ARTIFACTS := \
	harness/fixtures/out/Counter.sol/Counter.json \
	harness/fixtures/out/FixtureDeployer.sol/FixtureDeployer.json \
	harness/fixtures/out/MockGMP.sol/MockGMP.json \
	harness/fixtures/out/MockIFT.sol/MockIFT.json

doctor-e2e: $(FIXTURE_ARTIFACTS) ## Check e2e tools and SUT paths
	@command -v go >/dev/null || { echo "missing go" >&2; exit 1; }
	@command -v docker >/dev/null || { echo "missing docker; Docker is required for e2e lanes" >&2; exit 1; }
	@docker info >/dev/null || { echo "docker daemon is not reachable" >&2; exit 1; }
	@if [ -n "$$IBC_BIN" ]; then echo "IBC_BIN=$$IBC_BIN"; else echo "IBC_BIN=bin/ibc (default)"; fi
	@if [ -n "$$IBC_STUB_BIN" ]; then echo "IBC_STUB_BIN=$$IBC_STUB_BIN"; else echo "IBC_STUB_BIN=bin/ibc-stub (default)"; fi

build-stub: $(FIXTURE_ARTIFACTS) ## Build the e2e stub SUT into bin/ibc-stub
	go -C e2e build -o $(CURDIR)/bin/ibc-stub ./stub/cmd/ibc

$(FIXTURE_ARTIFACTS):
	@echo "missing fixture artifact $@ — run 'make fixtures' (requires forge)" >&2; exit 1

test-unit: ## Pure-Go tests for the harness and e2e/stub modules, no chains
	go -C harness test ./...
	go -C e2e test ./stub/...

test-e2e: build build-stub ## Run e2e tests (E2E_PKGS=... E2E_FLAGS=... E2E_LANE=...)
	E2E_LANE=$(E2E_LANE) go -C e2e test $(E2E_PKGS) $(E2E_FLAGS)

clean-e2e-dry-run: ## Preview ibc relayer run processes and e2e Docker resources
	scripts/clean-e2e.sh --dry-run

clean-e2e: ## Kill ibc relayer run processes and remove e2e Docker resources
	scripts/clean-e2e.sh

lint-e2e: ## Lint the harness and e2e modules
	cd harness && golangci-lint run
	cd e2e && golangci-lint run

fixtures: ## Rebuild embedded forge artifacts (requires forge)
	forge build --root harness/fixtures

check-fixtures: ## Fail if harness/fixtures/out is stale
	forge build --force --root harness/fixtures # --force: a warm cache would skip the rebuild and mask staleness
	@git diff --exit-code -- harness/fixtures/out || { \
		echo "harness/fixtures/out is stale — run 'make fixtures' and commit the result" >&2; exit 1; }

check: build test lint doctor-e2e test-unit lint-e2e check-fixtures test-e2e ## Root build/test/lint + preflight + unit + lint + fixtures + smoke e2e

.PHONY: help build lint lint-fix test codegen codegen-sql codegen-proto codegen-mocks \
	doctor-e2e build-stub test-unit test-e2e clean-e2e clean-e2e-dry-run lint-e2e fixtures \
	check-fixtures check
