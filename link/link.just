# SPDX-License-Identifier: Apache-2.0

# Recipes for IBC Link. Invoked from the repository root justfile as the
# `link` module; recipes run with `link/` as the working directory.

set dotenv-load

sqlc_version := env("SQLC_VERSION", "1.30.0")
buf_version := env("BUF_VERSION", "1.70.0")
ci := env("CI", "false")
sqlc := env("SQLC", if ci == "true" { "sqlc" } else { "go run github.com/sqlc-dev/sqlc/cmd/sqlc@v" + sqlc_version })
buf := env("BUF", if ci == "true" { "buf" } else { "go run github.com/bufbuild/buf/cmd/buf@v" + buf_version })

# List Link recipes
default:
    just --list link

# Build the Link binary
[group('build')]
build:
    mkdir -p bin
    go build -o bin/ibc ./cmd/ibc/...

# Install the Link binary
[group('build')]
install:
    go install ./cmd/ibc/...

# Build the local Docker image
[group('build')]
docker-build:
    docker build --target target-builder -t ibc:latest .

# Lint Link
[group('lint')]
lint:
    golangci-lint run

# Lint Link and fix errors
[group('lint')]
lint-fix:
    golangci-lint run --fix

# Run Link tests
[group('test')]
test:
    go test ./... -v

# Generate all Link code
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

# Check generated Link code is current
[group('check')]
check-stale:
    #!/usr/bin/env bash
    set -e
    echo "Checking for stale proto bindings..."
    just --justfile ../justfile link::codegen-proto
    status="$(git -C .. status --porcelain --untracked-files=all -- \
      ':(glob)link/api/v2/**/*.pb.go' \
      ':(glob)link/api/v2/**/*.connect.go')"
    test -z "$status" || {
      echo "Link API bindings are stale — run 'just link::codegen-proto' and commit the result" >&2
      echo "$status" >&2
      exit 1
    }
    test ! -e ../api || { echo "the removed standalone API module still exists" >&2; exit 1; }
    old_api='github.com/cosmos/ibc/'api
    ! git --no-pager -C .. grep -n -F "$old_api" -- . || {
      echo "the removed API module is still referenced" >&2
      exit 1
    }

# Run all Link checks
[group('check')]
run-all-checks:
    @echo "==== 1. Linting ===="
    just --justfile ../justfile link::lint
    @echo "==== 2. Checking for stale code ===="
    just --justfile ../justfile link::check-stale
    @echo "==== 3. Building ===="
    just --justfile ../justfile link::build
    @echo "==== 4. Testing ===="
    just --justfile ../justfile link::test
