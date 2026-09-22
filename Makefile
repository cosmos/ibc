# SPDX-License-Identifier: Apache-2.0

help: ## List repository commands
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

LICENSE_EYE_VERSION ?= 0.8.0
GOVULNCHECK_VERSION ?= 1.8.0

# Generated code has no Makefile of its own: gen/ holds generated output only.
GEN_SOLIDITY_ABI_DIR := gen/go/solidity-abi

# Every Go module in the repository. There is deliberately no module at the root, so
# anything that scans "the Go code" has to iterate these.
GO_MODULE_DIRS := cli e2e $(GEN_SOLIDITY_ABI_DIR)

lint-license: ## Check SPDX license headers
	go run github.com/apache/skywalking-eyes/cmd/license-eye@v$(LICENSE_EYE_VERSION) \
		--config .licenserc.yaml header check

lint-fix-license: ## Add missing SPDX license headers
	go run github.com/apache/skywalking-eyes/cmd/license-eye@v$(LICENSE_EYE_VERSION) \
		--config .licenserc.yaml header fix

lint-gen: ## Lint generated Solidity Go binding packages
	cd $(GEN_SOLIDITY_ABI_DIR) && golangci-lint run

lint-fix-gen: ## Lint generated Solidity Go binding packages and fix errors
	cd $(GEN_SOLIDITY_ABI_DIR) && golangci-lint run --fix

test-gen: ## Compile generated Solidity Go binding packages
	go -C $(GEN_SOLIDITY_ABI_DIR) test ./...

vulncheck: ## Report known vulnerabilities in Go dependencies
	@bindir="$$(mktemp -d)"; \
	trap 'rm -rf "$$bindir"' EXIT INT TERM; \
	GOBIN="$$bindir" go install \
		golang.org/x/vuln/cmd/govulncheck@v$(GOVULNCHECK_VERSION) || exit $$?; \
	status=0; \
	for dir in $(GO_MODULE_DIRS); do \
		echo "==> govulncheck $$dir"; \
		(cd $$dir && "$$bindir/govulncheck" ./...); \
		code=$$?; \
		if [ $$code -ne 0 ] && [ $$code -ne 3 ]; then \
			echo "govulncheck could not scan $$dir (exit $$code)" >&2; \
			status=$$code; \
		fi; \
	done; \
	exit $$status

run-all-checks: ## Run "all-in-one" code validation step.
	$(MAKE) -C cli run-all-checks
	$(MAKE) -C e2e run-all-checks
	$(MAKE) lint-gen
	$(MAKE) test-gen
	$(MAKE) lint-license

.PHONY: help lint-license lint-fix-license lint-gen lint-fix-gen test-gen vulncheck run-all-checks
