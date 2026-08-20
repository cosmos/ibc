# SPDX-License-Identifier: Apache-2.0

help: ## List repository commands
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

LICENSE_EYE_VERSION ?= 0.8.0

# Generated code has no Makefile of its own: gen/ holds generated output only.
GEN_SOLIDITY_ABI_DIR := gen/go/solidity-abi

lint-license: ## Check SPDX license headers
	go run github.com/apache/skywalking-eyes/cmd/license-eye@v$(LICENSE_EYE_VERSION) \
		--config .licenserc.yaml header check

lint-gen: ## Lint generated Solidity Go binding packages
	cd $(GEN_SOLIDITY_ABI_DIR) && golangci-lint run

lint-fix-gen: ## Lint generated Solidity Go binding packages and fix errors
	cd $(GEN_SOLIDITY_ABI_DIR) && golangci-lint run --fix

test-gen: ## Compile generated Solidity Go binding packages
	go -C $(GEN_SOLIDITY_ABI_DIR) test ./...

run-all-checks: ## Run "all-in-one" code validation step.
	$(MAKE) -C link run-all-checks
	$(MAKE) -C e2e run-all-checks
	$(MAKE) lint-gen
	$(MAKE) test-gen
	$(MAKE) lint-license

.PHONY: help lint-license lint-gen lint-fix-gen test-gen run-all-checks
