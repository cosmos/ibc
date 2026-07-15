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

lint: ## Lint the code
	golangci-lint run

lint-fix: ## Lint the code and fix errors
	golangci-lint run --fix

lint-stub-tests: ## Lint the temporary stub tests without changing Link's general test-lint policy
	golangci-lint run --tests ./internal/stub/...

codegen: codegen-sql codegen-proto codegen-mocks ## Generate code

codegen-sql: ## Generate Go code based on migrations and queries
	cd internal/store && $(SQLC) generate

codegen-proto: ## Generate Go proto bindings
	cd ../proto && $(BUF) generate

check-proto: ## Fail if Link's generated API is stale or the removed API module returns
	@command -v buf >/dev/null || { echo "missing buf; buf is required to verify Link's generated API" >&2; exit 1; }
	cd ../proto && buf generate
	@status="$$(git -C .. status --porcelain --untracked-files=all -- \
		':(glob)link/api/v2/**/*.pb.go' \
		':(glob)link/api/v2/**/*.connect.go')"; \
		test -z "$$status" || { echo "Link API bindings are stale — run 'make codegen-proto' and commit the result" >&2; echo "$$status" >&2; exit 1; }
	@test ! -e ../api || { echo "the removed standalone API module still exists" >&2; exit 1; }
	@old_api='github.com/cosmos/ibc/'api; \
		! git -C .. grep -n -F "$$old_api" -- . || { echo "the removed API module is still referenced" >&2; exit 1; }

codegen-mocks: ## Generate mocks for the code
	go run github.com/vektra/mockery/v3@v3.7.1

test: ## Run tests
	go test ./... -v

check: build test lint lint-stub-tests check-proto ## Run Link build, tests, lint, and generation freshness

.PHONY: help build lint lint-fix lint-stub-tests test codegen codegen-sql codegen-proto codegen-mocks check-proto check
