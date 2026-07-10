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

E2E_PKGS ?= ./setup ./ibclink ./external ./cosmos
E2E_FLAGS ?= -count=1
E2E_LANE ?= anvil

FIXTURE_ARTIFACTS := \
	harness/fixtures/out/Counter.sol/Counter.json \
	harness/fixtures/out/FixtureDeployer.sol/FixtureDeployer.json \
	harness/fixtures/out/MockGMP.sol/MockGMP.json \
	harness/fixtures/out/MockIFT.sol/MockIFT.json

# sandbox-ledger is the real Cosmos SDK node used by cosmos tests and the sandbox lane.
# It is pinned, cached under .deps/, and built into bin/sandboxd. SANDBOXD_BIN overrides it.
SANDBOXD_REPO := https://github.com/cosmos/sandbox-ledger.git
SANDBOXD_SHA  := 01a55e003b5490896a60c6c0b66e985f7fa7e16f
SANDBOXD_SRC  := .deps/sandbox-ledger
SANDBOXD_OUT  := bin/sandboxd

doctor-e2e: $(FIXTURE_ARTIFACTS) ## Check e2e tools, SUT paths, and sandboxd status
	@command -v go >/dev/null || { echo "missing go" >&2; exit 1; }
	@command -v docker >/dev/null || { echo "missing docker; Docker is required for e2e lanes" >&2; exit 1; }
	@docker info >/dev/null || { echo "docker daemon is not reachable" >&2; exit 1; }
	@if [ -n "$$IBC_BIN" ]; then echo "IBC_BIN=$$IBC_BIN"; else echo "IBC_BIN=bin/ibc (default)"; fi
	@if [ -n "$$IBC_STUB_BIN" ]; then echo "IBC_STUB_BIN=$$IBC_STUB_BIN"; else echo "IBC_STUB_BIN=bin/ibc-stub (default)"; fi
	@if [ -n "$$SANDBOXD_BIN" ]; then echo "SANDBOXD_BIN=$$SANDBOXD_BIN"; \
	elif [ -x $(SANDBOXD_OUT) ]; then echo "sandboxd: $(SANDBOXD_OUT) (built)"; \
	else echo "sandboxd: missing — needed by the smoke suite's e2e/cosmos tests and E2E_LANE=sandbox ('make test-e2e' builds it)"; fi

build-stub: $(FIXTURE_ARTIFACTS) ## Build the e2e stub SUT into bin/ibc-stub
	go -C e2e build -o $(CURDIR)/bin/ibc-stub ./stub/cmd/ibc

$(SANDBOXD_OUT):
	@echo "--- fetching sandbox-ledger @ $(SANDBOXD_SHA) into $(SANDBOXD_SRC)"
	@mkdir -p $(SANDBOXD_SRC)
	@if [ ! -d $(SANDBOXD_SRC)/.git ]; then git -C $(SANDBOXD_SRC) init -q; fi
	@git -C $(SANDBOXD_SRC) fetch -q --depth 1 $(SANDBOXD_REPO) $(SANDBOXD_SHA)
	@git -C $(SANDBOXD_SRC) checkout -q -f FETCH_HEAD
	@echo "--- building sandboxd -> $(SANDBOXD_OUT)"
	@mkdir -p bin
	cd $(SANDBOXD_SRC) && go build -o $(CURDIR)/$(SANDBOXD_OUT) ./cmd/sandboxd
	@echo "--- sandboxd built: $(SANDBOXD_OUT) ($$($(CURDIR)/$(SANDBOXD_OUT) version 2>/dev/null | tail -1))"

$(FIXTURE_ARTIFACTS):
	@echo "missing fixture artifact $@ — run 'make fixtures' (requires forge)" >&2; exit 1

test-unit: ## Pure-Go tests for the harness and e2e/stub modules, no chains
	go -C harness test ./...
	go -C e2e test ./stub/...

test-e2e: build build-stub $(SANDBOXD_OUT) ## Run e2e tests (E2E_PKGS=... E2E_FLAGS=... E2E_LANE=...)
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
	forge test --force --root harness/fixtures
	@git diff --exit-code -- harness/fixtures/out || { \
		echo "harness/fixtures/out is stale — run 'make fixtures' and commit the result" >&2; exit 1; }

check: build test lint doctor-e2e test-unit lint-e2e check-fixtures test-e2e ## Root build/test/lint + preflight + unit + lint + fixtures + smoke e2e

.PHONY: help build lint lint-fix test codegen codegen-sql codegen-proto codegen-mocks \
	doctor-e2e build-stub test-unit test-e2e clean-e2e clean-e2e-dry-run lint-e2e fixtures \
	check-fixtures check
