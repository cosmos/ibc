#!/bin/sh
set -eu

repo_root=$(CDPATH=''; cd -- "$(dirname -- "$0")/../.." && pwd)
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

generate() {
	artifact=$1
	package=$2
	type_name=$3
	output=$4
	abi_file="$tmp_dir/$type_name.abi"
	bin_file="$tmp_dir/$type_name.bin"

	jq -c '.abi' "$artifact" > "$abi_file"
	jq -r '.bytecode.object' "$artifact" > "$bin_file"
	abigen --abi "$abi_file" --bin "$bin_file" --pkg "$package" --type "$type_name" --out "$output"
}

test_apps="$repo_root/e2e/internal/testapp/contracts"
bindings="$test_apps/bindings"
stub="$repo_root/link/internal/stub"
mkdir -p "$bindings" "$stub"
generate "$test_apps/out/Counter.sol/Counter.json" bindings Counter "$bindings/Counter.go"
generate "$test_apps/out/MockGMP.sol/MockGMP.json" bindings MockGMP "$bindings/MockGMP.go"
generate "$test_apps/out/MockIFT.sol/MockIFT.json" bindings MockIFT "$bindings/MockIFT.go"
generate "$test_apps/out/TestAppDeployer.sol/TestAppDeployer.json" bindings TestAppDeployer "$bindings/TestAppDeployer.go"
generate "$test_apps/out/Counter.sol/Counter.json" stub Counter "$stub/Counter.go"
generate "$test_apps/out/MockGMP.sol/MockGMP.json" stub MockGMP "$stub/MockGMP.go"
generate "$test_apps/out/MockIFT.sol/MockIFT.json" stub MockIFT "$stub/MockIFT.go"
generate "$test_apps/out/TestAppDeployer.sol/TestAppDeployer.json" stub TestAppDeployer "$stub/TestAppDeployer.go"

solidity_ibc="$repo_root/e2e/internal/harness/environment/solidityibc"
access_manager="$solidity_ibc/accessmanager"
mkdir -p "$access_manager"
generate "$solidity_ibc/contracts/out/AccessManager.sol/AccessManager.json" accessmanager AccessManager "$access_manager/contract.go"
