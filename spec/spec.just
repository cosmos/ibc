# SPDX-License-Identifier: Apache-2.0

# Recipes for IBC specifications. Invoked from the repository root justfile as
# the `spec` module; recipes run with `spec/` as the working directory.

# List specification recipes
default:
    just --list spec

# Lint specifications
[group('lint')]
lint:
    markdownlint-cli2 "**.md"

# Lint specifications and fix errors
[group('lint')]
lint-fix:
    markdownlint-cli2 "**.md" --fix
