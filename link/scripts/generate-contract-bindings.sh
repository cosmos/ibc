#!/bin/sh
set -eu

link_root=$(
  CDPATH=''
  cd -- "$(dirname -- "$0")/.." && pwd
)
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

# abigen comes from the module's tool directive; artifacts come from the
# deploy driver's forge workspace, built by `make codegen-bindings`.
generate() {
  artifact=$1
  package=$2
  type_name=$3
  output=$4
  abi_file="$tmp_dir/$type_name.abi"
  bin_file="$tmp_dir/$type_name.bin"

  mkdir -p "$(dirname "$output")"
  jq -c '.abi' "$artifact" >"$abi_file"
  jq -r '.bytecode.object' "$artifact" >"$bin_file"
  go -C "$link_root" tool abigen --abi "$abi_file" --bin "$bin_file" --pkg "$package" --type "$type_name" --out "$output"
}

forge_out="$link_root/internal/deploy/evm/forgeproject/out"
generate "$forge_out/AccessManager.sol/AccessManager.json" accessmanager AccessManager "$link_root/internal/deploy/evm/accessmanager/contract.go"
