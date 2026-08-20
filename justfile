# SPDX-License-Identifier: Apache-2.0

set dotenv-load

# IBC Link recipes (run from the link directory)
mod link 'link/link.just'

# Repository E2E recipes (run from the e2e directory)
mod e2e 'e2e/e2e.just'

# IBC specification recipes (run from the spec directory)
mod spec 'spec/spec.just'

license_eye_version := env("LICENSE_EYE_VERSION", "0.8.0")
gen_solidity_abi_dir := "gen/go/solidity-abi"

# List all available recipes
default:
  just --list

# Check SPDX license headers
[group('lint')]
lint-license:
  go run github.com/apache/skywalking-eyes/cmd/license-eye@v{{license_eye_version}} \
    --config .licenserc.yaml header check

# Lint generated Solidity Go binding packages
[group('lint')]
lint-gen:
  cd {{gen_solidity_abi_dir}} && golangci-lint run

# Lint generated Solidity Go binding packages and fix errors
[group('lint')]
lint-fix-gen:
  cd {{gen_solidity_abi_dir}} && golangci-lint run --fix

# Compile generated Solidity Go binding packages
[group('test')]
test-gen:
  go -C {{gen_solidity_abi_dir}} test ./...

# Run all repository checks
[group('check')]
run-all-checks:
  just link::run-all-checks
  just e2e::run-all-checks
  just lint-gen
  just test-gen
  just lint-license
