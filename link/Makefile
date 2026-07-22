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
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary
	go build -o bin/ibc ./cmd/ibc/...

lint: ## Lint the code
	golangci-lint run

lint-fix: ## Lint the code and fix errors
	golangci-lint run --fix

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

test: ## Run tests
	go test ./... -v

.PHONY: help build lint lint-fix test codegen codegen-sql codegen-proto codegen-mocks codegen-abi
