# SPDX-License-Identifier: Apache-2.0

SQLC_VERSION ?= 1.30.0
BUF_VERSION ?= 1.70.0

CI ?= false

ifeq ($(CI),true)
$(info Makefile: using CI mode)
## Will use binaries from PATH
SQLC ?= sqlc
BUF ?= buf
else
SQLC ?= go run github.com/sqlc-dev/sqlc/cmd/sqlc@v$(SQLC_VERSION)
BUF ?= go run github.com/bufbuild/buf/cmd/buf@v$(BUF_VERSION)
endif

help: ## List of commands
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary
	mkdir -p bin
	go build -o bin/ibc ./cmd/ibc/...

build-test-bin: ## Build the binary used only by tests
	mkdir -p bin
	go build -o bin/testprover ./internal/testutil/proverservice/cmd/...

install: ## Install the binary
	go install ./cmd/ibc/...

docker-build: ## Build the local Docker image
	docker build --target target-builder -t ibc:latest .

lint: ## Lint the code
	golangci-lint run

lint-fix: ## Lint the code and fix errors
	golangci-lint run --fix

test: ## Run tests
	go test ./... -v

codegen: codegen-sql codegen-proto codegen-abi codegen-mocks ## Generate code

codegen-sql: ## Generate Go code based on migrations and queries
	cd internal/store && $(SQLC) generate

codegen-proto: ## Generate Go proto bindings
	cd ../proto && $(BUF) generate

codegen-mocks: ## Generate mocks for the code
	go run github.com/vektra/mockery/v3@v3.7.1 --log-level ERROR

# todo: replace the copied ABI files and this target with the contracts' own build
# outputs and go-abigen bindings once they are migrated from solidity-ibc-eureka
codegen-abi: ## Generate Go bindings for EVM contract ABIs
	mkdir -p internal/chains/evm/contracts/ics26router
	go tool abigen \
		--abi internal/chains/evm/abi/ics26_router.json \
		--pkg ics26router \
		--type Contract \
		--out internal/chains/evm/contracts/ics26router/ics26router.go
	mkdir -p internal/chains/evm/contracts/attestation
	go tool abigen \
		--abi internal/chains/evm/abi/attestation_light_client.json \
		--pkg attestation \
		--type Contract \
		--out internal/chains/evm/contracts/attestation/attestation.go

check-stale: ## Run all checks for stale code
	@echo "Checking for stale proto bindings..."
	$(MAKE) codegen-proto
	@status="$$(git -C .. status --porcelain --untracked-files=all -- \
		':(glob)link/api/v2/**/*.pb.go' \
		':(glob)link/api/v2/**/*.connect.go')"; \
		test -z "$$status" || { echo "Link API bindings are stale — run 'make codegen-proto' and commit the result" >&2; echo "$$status" >&2; exit 1; }
	@test ! -e ../api || { echo "the removed standalone API module still exists" >&2; exit 1; }
	@old_api='github.com/cosmos/ibc/'api; \
		! git --no-pager -C .. grep -n -F "$$old_api" -- . || { echo "the removed API module is still referenced" >&2; exit 1; }

run-all-checks: ## Run "all-in-one" code validation step.
	@echo "==== 1. Linting ===="
	$(MAKE) lint

	@echo "==== 2. Checking for stale code ===="
	$(MAKE) check-stale

	@echo "==== 3. Building ===="
	$(MAKE) build

	@echo "==== 4. Testing ===="
	$(MAKE) test


.PHONY: build-test-bin
.PHONY: help build install docker-build lint lint-fix test
.PHONY: codegen codegen-sql codegen-proto codegen-abi codegen-mocks
.PHONY: check-stale run-all-checks
