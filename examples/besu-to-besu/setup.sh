#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0

# setup.sh — besu-to-besu example.
#
# Boots two independent single-validator Besu QBFT chains (A and B), the
# substrate for a single IBC pair:
#
#       A  ◀──IBC──▶  B
#
# Every secret is derived from a BIP-39 mnemonic at init time — no private key
# is committed to the repo. `init` derives each chain's validator key, computes
# the matching QBFT genesis extraData, and renders besu.toml / el-genesis.json
# into chains/local/<chain>/, which is what docker-compose bind-mounts.
#
# Usage:
#   ./setup.sh              — init + start + wait for RPC (default end-to-end)
#   ./setup.sh init         — derive validator keys and render chain configs
#                             into chains/local/. Touches no containers.
#   ./setup.sh start        — docker compose up both chains, wait for RPC
#                             (init must have run)
#   ./setup.sh accounts     — print the derived accounts and their roles
#   ./setup.sh status       — RPC endpoints, chain IDs, block heights
#   ./setup.sh clean        — stop containers, remove volumes and chains/local/
#
# Environment (optional):
#   A_MNEMONIC           BIP-39 phrase every chain A account derives from
#   B_MNEMONIC           BIP-39 phrase every chain B account derives from
#                        (each chain has its own phrase, distinct from the
#                        other's, so the chains share no accounts)
#   A_VALIDATOR_INDEX    index of chain A's validator within A_MNEMONIC (1)
#   B_VALIDATOR_INDEX    index of chain B's validator within B_MNEMONIC (1)
#                        Index 0 of each phrase is that chain's deployer and is
#                        deliberately not a validator anywhere.
#   FUNDED_ACCOUNTS      how many accounts from each chain's phrase to pre-fund
#                        in its genesis (default 5). init logs every address.
#   GENESIS_BALANCE      hex wei per funded account (default 1 000 000 ETH)
#   QBFT_BLOCK_PERIOD_SECONDS / QBFT_EPOCH_LENGTH /
#   QBFT_REQUEST_TIMEOUT_SECONDS   QBFT consensus tunables
#
# Changing any of the above invalidates the on-disk chain data: run
# './setup.sh clean' before re-running, or Besu will refuse to start against a
# genesis that no longer matches its database.
#
# Requirements: docker (compose plugin), curl, perl. `cast` is used for key
# derivation — the foundry image is used automatically when it is not on PATH.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"
LIB_DIR="$SCRIPT_DIR/lib"
CHAINS_DIR="$SCRIPT_DIR/chains"
LOCAL_DIR="$CHAINS_DIR/local"
CHAINS_ENV_FILE="$LOCAL_DIR/chains.env"

LOG_DIR="$SCRIPT_DIR/logs"
mkdir -p "$LOG_DIR"
LOG_FILE="$LOG_DIR/setup-$(date '+%Y%m%d-%H%M%S').log"
exec > >(tee >(perl -pe 's/\x1b\[[0-9;]*[A-Za-z]//g' >> "$LOG_FILE")) 2>&1
echo "[$(date '+%H:%M:%S')] Logging to $LOG_FILE"

export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-$(basename "$SCRIPT_DIR")}"
export BESU_IMAGE="${BESU_IMAGE:-hyperledger/besu:25.4.0}"
export FOUNDRY_IMAGE="${FOUNDRY_IMAGE:-ghcr.io/foundry-rs/foundry:latest}"

# Key material. One mnemonic per chain, both publicly known devnet phrases.
# Every account on a chain — deployer, validator, and the rest of the pre-funded
# set — derives from that chain's phrase, so the two chains share no accounts.
#
# The phrases are deliberately distinct rather than two indices of one mnemonic.
# A QBFT light client trusts the counterparty's validator set by address, so if
# A and B shared a validator, a header signed for one chain would be
# signature-valid against the other's client. Distinct phrases make that
# impossible by construction.
#
# Never point either of these at a mnemonic holding real funds: derived
# validator keys are written to chains/local/<chain>/key in plaintext for Besu
# to read.
export A_MNEMONIC="${A_MNEMONIC:-legal winner thank year wave sausage worth useful legal winner thank yellow}"
export B_MNEMONIC="${B_MNEMONIC:-letter advice cage absurd amount doctor acoustic avoid letter advice cage above}"
# Index 0 is the deployer on both chains and index 1 is the validator on both,
# so no chain's block proposer is also the account sending txs. A validator is
# its chain's coinbase and pockets the priority fee, which would otherwise make
# an identical tx cost the deployer less on its own chain than on the other.
export A_VALIDATOR_INDEX="${A_VALIDATOR_INDEX:-1}"
export B_VALIDATOR_INDEX="${B_VALIDATOR_INDEX:-1}"
export FUNDED_ACCOUNTS="${FUNDED_ACCOUNTS:-5}"
export GENESIS_BALANCE="${GENESIS_BALANCE:-0xd3c21bcecceda1000000}"  # 1e24 wei = 1M ETH

# QBFT consensus tunables — substituted into el-genesis.json.tmpl.
export QBFT_BLOCK_PERIOD_SECONDS="${QBFT_BLOCK_PERIOD_SECONDS:-2}"
export QBFT_EPOCH_LENGTH="${QBFT_EPOCH_LENGTH:-30000}"
export QBFT_REQUEST_TIMEOUT_SECONDS="${QBFT_REQUEST_TIMEOUT_SECONDS:-4}"

# shellcheck source=lib/common.sh
source "$LIB_DIR/common.sh"
# shellcheck source=lib/chains.sh
source "$LIB_DIR/chains.sh"

cmd_init() {
  check_prerequisites
  log "--- Phase 1: Derive keys + render chain configs ---"
  # init_chains logs each chain's funded alloc set as it renders, so there is
  # no print_accounts call here — that would restate the same addresses.
  init_chains
}

cmd_start() {
  run_phase "Phase 2A: Start chains"  start_chains
  run_phase "Phase 2B: Wait for RPC"  wait_for_chains
  print_status
  log "Chains are live and producing blocks."
}

main() {
  case "${1:-}" in
    init)     cmd_init;       exit 0 ;;
    start)    cmd_start;      exit 0 ;;
    accounts) print_accounts; exit 0 ;;
    status)   print_status;   exit 0 ;;
    clean)    clean;          exit 0 ;;
    "")
      log "╔══════════════════════════════════════════════════╗"
      log "║  besu-to-besu: 2 Besu QBFT chains (A, B)         ║"
      log "╚══════════════════════════════════════════════════╝"
      cmd_init
      cmd_start
      exit 0
      ;;
    *)
      echo "Usage: $0 [init|start|accounts|status|clean]" >&2
      exit 1
      ;;
  esac
}

main "$@"
