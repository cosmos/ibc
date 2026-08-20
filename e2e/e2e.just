# SPDX-License-Identifier: Apache-2.0

# Recipes for repository E2E tests. Invoked from the repository root justfile
# as the `e2e` module; recipes run with `e2e/` as the working directory.

set dotenv-load

e2e_flags := env("E2E_FLAGS", "-count=1")
e2e_mode := env("E2E_MODE", "fast")
forge_build_flags := env("FORGE_BUILD_FLAGS", "")
contract_bindings := "../gen/go/solidity-abi/accessmanager ../gen/go/solidity-abi/escrow ../gen/go/solidity-abi/testerc20 ../gen/go/solidity-abi/counter ../gen/go/solidity-abi/iftsendcallconstructor ../gen/go/solidity-abi/iftbatchtransfershim"

# List E2E recipes
default:
    just --list e2e

# Check runtime dependencies used by E2E tests
[group('check')]
doctor:
    @command -v go >/dev/null || { echo "missing go" >&2; exit 1; }
    @command -v docker >/dev/null || { echo "missing docker; Docker is required for e2e modes and matrix generation" >&2; exit 1; }
    @docker info >/dev/null || { echo "docker daemon is not reachable" >&2; exit 1; }

# Check generation and lint tools used by E2E checks
[group('check')]
doctor-tools:
    @command -v forge >/dev/null || { echo "missing forge; Forge is required to verify test-app artifacts" >&2; exit 1; }
    @command -v bun >/dev/null || { echo "missing bun; bun is required to install Solidity contract dependencies" >&2; exit 1; }
    @command -v abigen >/dev/null || { echo "missing abigen; abigen is required to generate typed contract bindings" >&2; exit 1; }
    @command -v jq >/dev/null || { echo "missing jq; jq is required to generate typed contract bindings" >&2; exit 1; }
    @command -v golangci-lint >/dev/null || { echo "missing golangci-lint; it is required for e2e checks" >&2; exit 1; }

# Lint E2E code
[group('lint')]
lint:
    golangci-lint run

# Lint E2E code and fix errors
[group('lint')]
lint-fix:
    golangci-lint run --fix

# Run tests for the E2E harness
[group('test')]
test-harness: _build-link
    go test ./internal/... ./cmd/... -count=1

# Rebuild test-app artifacts and typed Go bindings
[group('test')]
test-apps:
    bun install --cwd internal/harness/environment/solidityibc/contracts --frozen-lockfile
    forge build {{ forge_build_flags }} --root internal/harness/environment/solidityibc/contracts
    ../scripts/generate-contract-bindings.sh

# Run E2E tests
[group('test')]
test: _build-link
    # -parallel caps concurrent Docker environments; the GOMAXPROCS default can overload a large machine.
    E2E_MODE={{ e2e_mode }} go test . -timeout 60m -parallel 4 {{ e2e_flags }}

# Regenerate the E2E provider and topology matrix
[group('generate')]
generate-matrix:
    go run ./cmd/e2e-matrix -write test-matrix.md

# Check the generated E2E matrix is current
[group('check')]
check-matrix:
    go run ./cmd/e2e-matrix -check test-matrix.md

# Kill E2E processes and remove Docker resources
[group('clean')]
clean:
    ./scripts/clean.sh

# Preview E2E processes and Docker resources
[group('clean')]
clean-dry-run:
    ./scripts/clean.sh --dry-run

# Check generated contract bindings are current
[group('check')]
check-stale:
    #!/usr/bin/env bash
    set -e
    echo "Checking for stale ABI Go bindings..."
    FORGE_BUILD_FLAGS=--force just --justfile ../justfile e2e::test-apps
    status="$(git status --porcelain --untracked-files=all -- {{ contract_bindings }})"
    test -z "$status" || {
      echo "contract bindings are stale — run 'just e2e::test-apps' and commit the result" >&2
      echo "$status" >&2
      exit 1
    }

# Run all E2E checks
[group('check')]
run-all-checks: doctor doctor-tools
    @echo "==== 1. Linting ===="
    just --justfile ../justfile e2e::lint
    @echo "==== 2. Checking for stale code ===="
    just --justfile ../justfile e2e::check-stale
    @echo "==== 3. Testing harness ===="
    just --justfile ../justfile e2e::test-harness
    @echo "==== 4. Running E2E ===="
    just --justfile ../justfile e2e::test
    @echo "==== 5. Checking matrix ===="
    just --justfile ../justfile e2e::check-matrix

[private]
_build-link:
    just --justfile ../justfile link::build
