# SPDX-License-Identifier: Apache-2.0

SQLC_VERSION ?= 1.30.0
BUF_VERSION ?= 1.70.0

CI ?= false

# The org's Go module prefix, such as github.com/<org>, taken from this module's path
# github.com/<org>/ibc/cli. Set GO_ORG in the environment to use another one.
GO_ORG ?= $(shell awk '$$1 == "module" { sub("/[^/]*/[^/]*$$", "", $$2); print $$2 }' go.mod)

# The ibc-contracts version pinned in cli/go.mod, reported by `ibc --version`.
CONTRACTS_VERSION := $(shell awk '$$1 == "$(GO_ORG)/ibc-contracts/packages/go-abigen" { print $$2 }' go.mod)

# The ibc-go version pinned in cli/go.mod, reported by `ibc --version`. The module path ends in its
# major version, such as /v11.
IBC_GO_VERSION := $(shell awk '$$1 ~ "^$(GO_ORG)/ibc-go(/v[0-9]+)?$$" { print $$2 }' go.mod)

VERSION_LDFLAGS := -X main.contractsVersion=$(CONTRACTS_VERSION) -X main.ibcGoVersion=$(IBC_GO_VERSION)

ifeq ($(CI),true)
# On stderr, so contracts-version and ibc-go-version print only the version.
$(shell echo "Makefile: using CI mode" >&2)
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
	go build -ldflags "$(VERSION_LDFLAGS)" -o bin/ibc ./cmd/ibc/...

build-test-bin: ## Build the binary used only by tests
	mkdir -p bin
	go build -o bin/testprover ./internal/testutil/proverservice/cmd/...

install: ## Install the binary
	go install -ldflags "$(VERSION_LDFLAGS)" ./cmd/ibc/...

contracts-version: ## Print the pinned ibc-contracts version
	@test -n "$(CONTRACTS_VERSION)" && echo "$(CONTRACTS_VERSION)"

ibc-go-version: ## Print the pinned ibc-go version
	@test -n "$(IBC_GO_VERSION)" && echo "$(IBC_GO_VERSION)"

docker-build: ## Build the local Docker image
	docker build --target target-builder -t ibc:latest .

lint: ## Lint the code
	golangci-lint run

lint-fix: ## Lint the code and fix errors
	golangci-lint run --fix

test: ## Run tests
	go test ./... -v

codegen: codegen-sql codegen-proto codegen-mocks ## Generate code

codegen-sql: ## Generate Go code based on migrations and queries
	cd internal/store && $(SQLC) generate

codegen-proto: ## Generate Go proto bindings
	cd ../proto && $(BUF) generate

codegen-mocks: ## Generate mocks for the code
	go run github.com/vektra/mockery/v3@v3.7.1 --log-level ERROR

check-stale: ## Run all checks for stale code
	@echo "Checking for stale proto bindings..."
	$(MAKE) codegen-proto
	@status="$$(git -C .. status --porcelain --untracked-files=all -- \
		':(glob)cli/api/v2/**/*.pb.go' \
		':(glob)cli/api/v2/**/*.connect.go')"; \
		test -z "$$status" || { echo "IBC CLI API bindings are stale — run 'make codegen-proto' and commit the result" >&2; echo "$$status" >&2; exit 1; }
	@test ! -e ../api || { echo "the removed standalone API module still exists" >&2; exit 1; }
	@old_api='$(GO_ORG)/ibc/'api; \
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
.PHONY: help build install contracts-version ibc-go-version docker-build lint lint-fix test
.PHONY: codegen codegen-sql codegen-proto codegen-mocks
.PHONY: check-stale run-all-checks
