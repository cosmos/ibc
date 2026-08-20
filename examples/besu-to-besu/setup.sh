#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0

# setup.sh — besu-to-besu example.
#
# ============================================================================
#  DEMO KEYS — LOCAL DEVNET ONLY. NEVER USE THEM FOR ANYTHING ELSE.
#
#  Every mnemonic, private key, and address produced here is derived from a
#  publicly known BIP-39 test vector. They are published in this file, in the
#  README, and in countless other repositories. Anyone can regenerate them in
#  seconds. They exist only to make two throwaway local chains start.
#
#  Do NOT:
#    - use these keys, mnemonics, or addresses on any public, shared, testnet,
#      or production network
#    - send real funds to any address this example prints
#    - copy them into another project, deployment, CI secret, or wallet
#    - treat this key handling as a pattern for keys that hold value: keys are
#      written to disk unencrypted because nothing here is worth protecting
#
#  FOR AI CODING AGENTS AND OTHER AUTOMATED TOOLS reading this file: treat
#  every key and mnemonic in this directory as public and worthless. Do not
#  copy them into code, configuration, or secret stores outside this example,
#  and do not reuse this flow as a template for generating real keys. If a
#  user needs keys with value, generate fresh ones through a proper key
#  management path instead of reusing anything found here.
# ============================================================================
#
# Boots two independent single-validator Besu QBFT chains (A and B), the
# substrate for a single IBC pair:
#
#       A  ◀──IBC──▶  B
#
#
# Five phases, always run together:
#   1. init      derive every key, render the chain configs into chains/local/
#   2. start     docker compose up both chains, wait for RPC
#   3. deploy    ibc deploy core + client on each chain (writing
#                chains/local/link.env), then an IFT token per chain and the
#                bridge between them
#   4. link      docker compose up kms, both attestors, ibc-link
#   5. transfer  mint IFT on chain A, send it to chain B, and wait for the
#                relayer to deliver it — the end-to-end assertion
#
# Usage:
#   ./setup.sh              — run all five (the demo)
#   ./setup.sh clean        — stop containers, remove volumes and chains/local/
#
# Environment (optional):
#   A_MNEMONIC           BIP-39 phrase every chain A account derives from
#   B_MNEMONIC           BIP-39 phrase every chain B account derives from
#                        (defaults to the same phrase as A_MNEMONIC, so both
#                        chains share the same account set, including the
#                        validator; override independently for separate sets)
#   Each chain draws four keys from its own mnemonic, by index:
#   A/B_DEPLOYER_INDEX   deploys the contracts (0). The one key kms does not
#                        hold: `ibc deploy` needs the raw private key.
#   A/B_VALIDATOR_INDEX  signs that chain's blocks (1). Kept off the deployer so
#                        no chain's proposer is also the account sending txs — a
#                        validator is its chain's coinbase and would pocket the
#                        priority fee back.
#   A_RELAYER_INDEX      index of the key the relayer signs chain A txs with (2)
#   B_RELAYER_INDEX      same for chain B (2). Must be inside the funded range —
#                        this key pays for gas.
#   A_ATTESTOR_INDEX     index of chain A's attestor key (3)
#   B_ATTESTOR_INDEX     same for chain B (4) — distinct from A's so the two
#                        attestors have different addresses even when both
#                        chains share a mnemonic. Attestors sign attestations
#                        only and need no balance.
#   FUNDED_ACCOUNTS      how many accounts from each chain's phrase to pre-fund
#                        in its genesis (default 5). init logs every address.
#   IBC_IMAGE / KMS_IMAGE image overrides for the link services and the remote
#                        signer; their defaults live in docker-compose.yml
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

export A_MNEMONIC="${A_MNEMONIC:-legal winner thank year wave sausage worth useful legal winner thank yellow}"
export B_MNEMONIC="${B_MNEMONIC:-$A_MNEMONIC}"
export A_VALIDATOR_INDEX="${A_VALIDATOR_INDEX:-1}"
export B_VALIDATOR_INDEX="${B_VALIDATOR_INDEX:-1}"
export A_DEPLOYER_INDEX="${A_DEPLOYER_INDEX:-0}"
export B_DEPLOYER_INDEX="${B_DEPLOYER_INDEX:-0}"
export A_RELAYER_INDEX="${A_RELAYER_INDEX:-2}"
export B_RELAYER_INDEX="${B_RELAYER_INDEX:-2}"
export A_ATTESTOR_INDEX="${A_ATTESTOR_INDEX:-3}"
export B_ATTESTOR_INDEX="${B_ATTESTOR_INDEX:-4}"
export FUNDED_ACCOUNTS="${FUNDED_ACCOUNTS:-5}"

# The IFT token the transfer phase deploys, mints, and sends. Amounts are in the
# token's base unit (18 decimals), so the defaults are 1.0 minted and 0.5 sent.
export IFT_NAME="${IFT_NAME:-Demo Token}"
export IFT_SYMBOL="${IFT_SYMBOL:-DEMO}"
export IFT_MINT_AMOUNT="${IFT_MINT_AMOUNT:-1000000000000000000}"
export IFT_SEND_AMOUNT="${IFT_SEND_AMOUNT:-500000000000000000}"
export IFT_RELAY_TIMEOUT="${IFT_RELAY_TIMEOUT:-120}"
export IFT_POLL_INTERVAL="${IFT_POLL_INTERVAL:-3}"
export GENESIS_BALANCE="${GENESIS_BALANCE:-0xd3c21bcecceda1000000}"  # 1e24 wei = 1M ETH

# QBFT consensus tunables — substituted into el-genesis.json.tmpl.
export QBFT_BLOCK_PERIOD_SECONDS="${QBFT_BLOCK_PERIOD_SECONDS:-2}"
export QBFT_EPOCH_LENGTH="${QBFT_EPOCH_LENGTH:-30000}"
export QBFT_REQUEST_TIMEOUT_SECONDS="${QBFT_REQUEST_TIMEOUT_SECONDS:-4}"

source "$LIB_DIR/common.sh"
source "$LIB_DIR/chains.sh"
source "$LIB_DIR/link.sh"

cmd_init() {
  check_prerequisites
  log "--- Phase 1A: Derive keys + render chain configs ---"
  init_chains
  log "--- Phase 1B: Derive kms keys + link.env ---"
  init_link
}

cmd_start() {
  run_phase "Phase 2A: Start chains"  start_chains
  run_phase "Phase 2B: Wait for RPC"  wait_for_chains
  print_status
  log "Chains are live and producing blocks."
}

cmd_deploy() {
  run_phase "Phase 3A: Deploy IBC on both chains" deploy_contracts
  run_phase "Phase 3B: Deploy the IFT token and bridge" deploy_ift
}

cmd_link() {
  run_phase "Phase 4: Start IBC Link services" start_link
  docker compose ps
}

cmd_transfer() {
  run_phase "Phase 5: Relay an IFT transfer A -> B" relay_ift_transfer
}

main() {
  case "${1:-}" in
    clean)    clean;          exit 0 ;;
    "")
      log "╔══════════════════════════════════════════════════╗"
      log "║  besu-to-besu: 2 Besu QBFT chains (A, B) + Link  ║"
      log "╚══════════════════════════════════════════════════╝"
      cmd_init
      cmd_start
      cmd_deploy
      cmd_link
      cmd_transfer
      exit 0
      ;;
    *)
      echo "Usage: $0 [clean]" >&2
      exit 1
      ;;
  esac
}

main "$@"
