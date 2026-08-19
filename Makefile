# SPDX-License-Identifier: Apache-2.0

help: ## List repository commands
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

LICENSE_EYE_VERSION ?= 0.8.0

lint-license: ## Check SPDX license headers
	go run github.com/apache/skywalking-eyes/cmd/license-eye@v$(LICENSE_EYE_VERSION) \
		--config .licenserc.yaml header check

# TODO
check: check-license-headers ## Run all repository checks
	$(MAKE) -C link check
	$(MAKE) -C e2e check

.PHONY: help lint-license
