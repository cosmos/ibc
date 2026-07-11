help: ## List of commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary
	go build -o bin/ibc ./cmd/ibc/...

lint: ## Lint the code
	golangci-lint run

lint-fix: ## Lint the code and fix errors
	golangci-lint run --fix

lint-e2e: ## Lint the e2e modules
	cd harness && golangci-lint run

lint-fix-e2e: ## Lint the e2e modules and fix errors
	cd harness && golangci-lint run --fix

codegen: codegen-sql codegen-proto codegen-mocks ## Generate code

codegen-sql: ## Generate Go code based on migrations and queries
	cd internal/store && sqlc generate

codegen-proto: ## Generate Go proto bindings
	cd ../proto && buf generate

codegen-mocks: ## Generate mocks for the code
	go run github.com/vektra/mockery/v3@v3.7.1

test: ## Run tests
	go test ./... -v

.PHONY: help build lint lint-fix lint-e2e lint-fix-e2e test codegen codegen-sql codegen-proto codegen-mocks
