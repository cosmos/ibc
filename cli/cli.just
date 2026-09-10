# SPDX-License-Identifier: Apache-2.0

# Recipes for the IBC CLI. Invoked from the repository root justfile as the
# `cli` module; recipes run with `cli/` as the working directory.

set dotenv-load

sqlc_version := env("SQLC_VERSION", "1.30.0")
buf_version := env("BUF_VERSION", "1.70.0")
ci := env("CI", "false")
sqlc := env("SQLC", if ci == "true" { "sqlc" } else { "go run github.com/sqlc-dev/sqlc/cmd/sqlc@v" + sqlc_version })
buf := env("BUF", if ci == "true" { "buf" } else { "go run github.com/bufbuild/buf/cmd/buf@v" + buf_version })

# List CLI recipes
default:
    just --list cli

# Build the CLI binary
[group('build')]
build:
    mkdir -p bin
    go build -o bin/ibc ./cmd/ibc/...

# Build the binary used only by tests
[group('build')]
build-test-bin:
    mkdir -p bin
    go build -o bin/testprover ./internal/testutil/proverservice/cmd/...

# Install the CLI binary
[group('build')]
install:
    go install ./cmd/ibc/...

# Build the local Docker image
[group('build')]
docker-build:
    docker build --target target-builder -t ibc:latest .

# Lint the CLI
[group('lint')]
lint:
    golangci-lint run

# Lint the CLI and fix errors
[group('lint')]
lint-fix:
    golangci-lint run --fix

# Run CLI tests
[group('test')]
test:
    go test ./... -v

# Generate all CLI code
[group('generate')]
codegen: codegen-sql codegen-proto codegen-abi codegen-mocks

# Generate Go code from migrations and queries
[group('generate')]
codegen-sql:
    cd internal/store && {{ sqlc }} generate

# Generate Go protobuf bindings
[group('generate')]
codegen-proto:
    cd ../proto && {{ buf }} generate

# Generate mocks
[group('generate')]
codegen-mocks:
    go run github.com/vektra/mockery/v3@v3.7.1 --log-level ERROR

# Generate Go bindings for EVM contract ABIs
[group('generate')]
codegen-abi:
    mkdir -p internal/chains/evm/contracts/ics26router
    go tool abigen \
      --abi internal/chains/evm/abi/ics26_router.json \
      --pkg ics26router \
      --type Contract \
      --out internal/chains/evm/contracts/ics26router/ics26router.go
    mkdir -p internal/chains/evm/contracts/attestation
    go tool abigen \
      --abi internal/chains/evm/abi/attestation_light_client.json \
      --pkg attestation \
      --type Contract \
      --out internal/chains/evm/contracts/attestation/attestation.go

# Check generated CLI code is current
[group('check')]
check-stale:
    #!/usr/bin/env bash
    set -e
    echo "Checking for stale proto bindings..."
    just --justfile ../justfile cli::codegen-proto
    status="$(git -C .. status --porcelain --untracked-files=all -- \
      ':(glob)cli/api/v2/**/*.pb.go' \
      ':(glob)cli/api/v2/**/*.connect.go')"
    test -z "$status" || {
      echo "IBC CLI API bindings are stale — run 'just cli::codegen-proto' and commit the result" >&2
      echo "$status" >&2
      exit 1
    }
    test ! -e ../api || { echo "the removed standalone API module still exists" >&2; exit 1; }
    old_api='github.com/cosmos/ibc/'api
    ! git --no-pager -C .. grep -n -F "$old_api" -- . || {
      echo "the removed API module is still referenced" >&2
      exit 1
    }

# Run all CLI checks
[group('check')]
run-all-checks:
    @echo "==== 1. Linting ===="
    just --justfile ../justfile cli::lint
    @echo "==== 2. Checking for stale code ===="
    just --justfile ../justfile cli::check-stale
    @echo "==== 3. Building ===="
    just --justfile ../justfile cli::build
    @echo "==== 4. Testing ===="
    just --justfile ../justfile cli::test
