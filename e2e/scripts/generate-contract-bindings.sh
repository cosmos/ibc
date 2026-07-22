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

	mkdir -p "$(dirname "$output")"
	jq -c '.abi' "$artifact" > "$abi_file"
	jq -r '.bytecode.object' "$artifact" > "$bin_file"
	abigen --abi "$abi_file" --bin "$bin_file" --pkg "$package" --type "$type_name" --out "$output"
}

solidity_ibc="$repo_root/e2e/internal/harness/environment/solidityibc"
generate "$solidity_ibc/contracts/out/AccessManager.sol/AccessManager.json" accessmanager AccessManager "$solidity_ibc/accessmanager/contract.go"
generate "$solidity_ibc/contracts/out/Escrow.sol/Escrow.json" escrow Escrow "$solidity_ibc/escrow/contract.go"
generate "$solidity_ibc/contracts/out/DummyLightClient.sol/DummyLightClient.json" dummylightclient DummyLightClient "$solidity_ibc/dummylightclient/contract.go"
generate "$solidity_ibc/contracts/out/TestERC20.sol/TestERC20.json" testerc20 TestERC20 "$solidity_ibc/testerc20/contract.go"
generate "$solidity_ibc/contracts/out/Counter.sol/Counter.json" counter Counter "$solidity_ibc/counter/contract.go"
generate "$solidity_ibc/contracts/out/EVMIFTSendCallConstructor.sol/EVMIFTSendCallConstructor.json" iftsendcallconstructor EVMIFTSendCallConstructor "$solidity_ibc/iftsendcallconstructor/contract.go"
