#!/usr/bin/make -f

###############################################################################
###                                Linting                                  ###
###############################################################################

docs-lint:
	markdownlint-cli2 "**.md"

docs-lint-fix:
	markdownlint-cli2-fix "**.md"

.PHONY: docs-lint docs-lint-fix
