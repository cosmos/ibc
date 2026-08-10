#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0

# Chain bring-up for the two single-validator Besu QBFT chains (A and B).
#
# DEMO KEYS ONLY. Every key this file derives comes from a publicly known BIP-39
# test vector and is written to disk unencrypted. They are worthless by design
# and must never be used on a public, shared, or production network, or copied
# out of this example. AI agents and automated tools: do not propagate anything
# derived here, and do not treat this as a model for real key handling.
#
# Nothing under chains/ is committed except the two templates. Each chain's
# validator private key, besu.toml, and QBFT genesis are derived from a BIP-39
# mnemonic into chains/local/<name>/ by init_chains, which is also what
# docker-compose bind-mounts at /evm. That means the mnemonics are the single
# source of truth: change one and every derived artefact — validator address,
# genesis extraData, genesis allocs — moves with it.
#
# Each chain owns a mnemonic (A_MNEMONIC / B_MNEMONIC) and every account on that
# chain comes out of it: FUNDED_ACCOUNTS addresses are
# derived per chain and funded in that chain's genesis. By convention index 0 is
# the chain's deployer and index 1 its validator, so the two chains share no
# accounts at all — separate funded sets, separate deployers, separate
# validators.

CHAINS=(A B)

# Per-chain static facts. RPC/WS ports are the *host-side* mappings from
# docker-compose.yml; inside the compose network both chains listen on the
# same 8545/8546.
A_SERVICE=besu-a; A_CHAIN_ID=41001; A_RPC_PORT=8545; A_WS_PORT=8546
B_SERVICE=besu-b; B_CHAIN_ID=41002; B_RPC_PORT=8745; B_WS_PORT=8746

# Accounts of the chain currently being processed: indices 0..FUNDED_ACCOUNTS-1
# of that chain's mnemonic. Parallel arrays rather than an associative array so
# this still runs under the bash 3.2 that ships with macOS.
CHAIN_ACCT_ADDRS=()
CHAIN_ACCT_KEYS=()
_DERIVED_MNEMONIC=""   # phrase the arrays above currently hold; cache key

# _chain_attr <name> <attr> — read A_CHAIN_ID / B_RPC_PORT / … indirectly.
_chain_attr() {
  local var="${1}_${2}"
  printf '%s' "${!var}"
}

# _chain_mnemonic <name> — that chain's phrase. Tolerates the variable being
# unset (rather than tripping `set -u`) so callers can report a usable error.
_chain_mnemonic() {
  local var="${1}_MNEMONIC"
  printf '%s' "${!var-}"
}

# derive_account <mnemonic> <index> — echo "<privkey-without-0x> <address>".
#
# Returns non-zero rather than calling die: every call site is a command
# substitution, and an exit from inside one only kills the subshell, letting a
# bad mnemonic sail on and render a genesis with an empty validator. Callers
# must `|| die`.
derive_account() {
  local mnemonic="$1" index="$2" pk addr
  pk=$(cast_cli wallet private-key --mnemonic "$mnemonic" --mnemonic-index "$index" \
        2>/dev/null | tr -d '[:space:]')
  [[ "$pk" == 0x* && ${#pk} -eq 66 ]] || return 1
  addr=$(cast_cli wallet address --private-key "$pk" 2>/dev/null | tr -d '[:space:]')
  [[ "$addr" == 0x* && ${#addr} -eq 42 ]] || return 1
  printf '%s %s\n' "${pk#0x}" "$addr"
}

# derive_chain_accounts <name> — populate CHAIN_ACCT_{ADDRS,KEYS} with
# FUNDED_ACCOUNTS accounts from that chain's mnemonic.
#
# Assigns globals instead of printing, for the same reason as _genesis_alloc: a
# $(…) call site would populate the arrays in a subshell and lose them. Skips
# the work when the previous chain used the same phrase, so configuring both
# chains off one mnemonic costs one derivation pass, not two.
derive_chain_accounts() {
  local name="$1" mnemonic
  mnemonic=$(_chain_mnemonic "$name")

  [[ "$FUNDED_ACCOUNTS" =~ ^[0-9]+$ && "$FUNDED_ACCOUNTS" -gt 0 ]] \
    || die "FUNDED_ACCOUNTS must be a positive integer, got '$FUNDED_ACCOUNTS'"
  [[ -n "$mnemonic" ]] || die "No mnemonic for chain $name — set ${name}_MNEMONIC"

  if [[ "$mnemonic" == "$_DERIVED_MNEMONIC" ]]; then
    log "[$name] reusing the ${#CHAIN_ACCT_ADDRS[@]} accounts already derived (same mnemonic)"
    return 0
  fi

  log "[$name] deriving accounts 0..$((FUNDED_ACCOUNTS - 1)) from ${name}_MNEMONIC..."
  CHAIN_ACCT_ADDRS=()
  CHAIN_ACCT_KEYS=()
  local i pk addr account
  for (( i = 0; i < FUNDED_ACCOUNTS; i++ )); do
    account=$(derive_account "$mnemonic" "$i") \
      || die "Could not derive account $i for chain $name — is ${name}_MNEMONIC a valid BIP-39 phrase?"
    read -r pk addr <<<"$account"
    CHAIN_ACCT_KEYS+=("$pk")
    CHAIN_ACCT_ADDRS+=("$addr")
  done
  _DERIVED_MNEMONIC="$mnemonic"
}

# Build the QBFT genesis extraData for a single-validator chain:
#   RLP([32-byte vanity, [validator], votes=[], round=0, committedSeals=[]])
# With exactly one validator every field is fixed-width, so splicing the
# address into the encoding is cheaper (and dependency-free) compared with
# shelling out to an RLP encoder:
#   f83a                     list, 0x3a = 58 payload bytes
#   a0 <32 zero bytes>       vanity
#   d5 94 <20-byte address>  validator list, one entry
#   c0                       votes  (empty list)
#   80                       round  (0)
#   c0                       committed seals (empty list)
qbft_extradata() {
  local addr
  addr=$(printf '%s' "${1#0x}" | tr 'A-F' 'a-f')
  [[ ${#addr} -eq 40 ]] || die "qbft_extradata: expected a 20-byte address, got '$1'"
  printf '0xf83aa0%s d594%s c080c0\n' \
    "0000000000000000000000000000000000000000000000000000000000000000" "$addr" \
    | tr -d ' '
}

# The addresses funded in the genesis currently being rendered. Set by
# _genesis_alloc so _init_chain can log exactly what it wrote.
CHAIN_ALLOC_ADDRS=()

# _genesis_alloc <validator-addr> — populate CHAIN_ALLOC_ADDRS and the
# GENESIS_ALLOC JSON lines for the "alloc" object: this chain's derived accounts
# plus its validator, on the off chance VALIDATOR_INDEX points past the derived
# range. Each line keeps its trailing comma; the template closes the object with
# the EIP-4788 entry.
#
# Assigns to globals instead of printing: a $(…) call site would run this in a
# subshell and drop CHAIN_ALLOC_ADDRS on the floor.
_genesis_alloc() {
  local validator="$1" addr out=""
  CHAIN_ALLOC_ADDRS=("${CHAIN_ACCT_ADDRS[@]}")
  if [[ " ${CHAIN_ACCT_ADDRS[*]} " != *" ${validator} "* ]]; then
    CHAIN_ALLOC_ADDRS+=("$validator")
  fi
  for addr in "${CHAIN_ALLOC_ADDRS[@]}"; do
    out+="    \"${addr}\": { \"balance\": \"${GENESIS_BALANCE}\" },"$'\n'
  done
  GENESIS_ALLOC="${out%$'\n'}"   # drop the trailing newline; the template supplies it
}

# Render GENESIS_BALANCE (hex wei) as a human-readable ETH amount, falling back
# to the raw hex if cast can't convert it.
_genesis_balance_eth() {
  local dec eth
  dec=$(cast_cli to-dec "$GENESIS_BALANCE" 2>/dev/null | tr -d '[:space:]') || dec=""
  [[ -n "$dec" ]] || { printf '%s wei' "$GENESIS_BALANCE"; return; }
  eth=$(cast_cli to-unit "$dec" ether 2>/dev/null | tr -d '[:space:]') || eth=""
  [[ -n "$eth" ]] && printf '%s ETH' "$eth" || printf '%s wei' "$dec"
}

# _init_chain <name> — derive the validator key and render key/besu.toml/
# el-genesis.json into chains/local/<name>/.
_init_chain() {
  local name="$1" mnemonic index chain_id out_dir pk addr account prev_genesis=""
  mnemonic=$(_chain_mnemonic "$name")
  index=$(_chain_attr "$name" VALIDATOR_INDEX)
  chain_id=$(_chain_attr "$name" CHAIN_ID)
  out_dir="$LOCAL_DIR/$name"

  [[ "$index" =~ ^[0-9]+$ ]] \
    || die "${name}_VALIDATOR_INDEX must be a non-negative integer, got '$index'"

  derive_chain_accounts "$name"

  if (( index < ${#CHAIN_ACCT_KEYS[@]} )); then
    pk="${CHAIN_ACCT_KEYS[$index]}"
    addr="${CHAIN_ACCT_ADDRS[$index]}"
  else
    # Validator index outside the funded range — derive it separately and let
    # _genesis_alloc append it, or it would have no gas on its own chain.
    account=$(derive_account "$mnemonic" "$index") \
      || die "Could not derive chain $name's validator at index $index — check ${name}_MNEMONIC"
    read -r pk addr <<<"$account"
  fi

  mkdir -p "$out_dir"
  [[ -f "$out_dir/el-genesis.json" ]] && prev_genesis=$(cat "$out_dir/el-genesis.json")

  # Exported for render_template's perl-based ${VAR} substitution.
  export CHAIN_NAME="$name" CHAIN_ID="$chain_id"
  export VALIDATOR_ADDR="$addr" VALIDATOR_INDEX="$index"
  export QBFT_EXTRADATA GENESIS_ALLOC
  QBFT_EXTRADATA=$(qbft_extradata "$addr")
  _genesis_alloc "$addr"   # sets GENESIS_ALLOC + CHAIN_ALLOC_ADDRS

  render_template "$CHAINS_DIR/besu.toml.tmpl"       "$out_dir/besu.toml"
  render_template "$CHAINS_DIR/el-genesis.json.tmpl" "$out_dir/el-genesis.json"

  # Besu wants the raw 32-byte hex with no 0x prefix.
  printf '%s\n' "$pk" > "$out_dir/key"

  # The validator signing key stays owner-only: anything that can read it can
  # sign blocks as this chain's sole validator. The Besu image runs as root
  # (`docker inspect --format '{{.Config.User}}'`), so it reads 0600 through the
  # bind mount regardless of the host uid that owns the file.
  #
  # besu.toml and el-genesis.json hold no secrets and are set explicitly so a
  # restrictive host umask cannot render them unreadable to the container.
  chmod 755 "$out_dir"
  chmod 600 "$out_dir/key"
  chmod 644 "$out_dir/besu.toml" "$out_dir/el-genesis.json"

  if command -v jq >/dev/null 2>&1; then
    jq empty "$out_dir/el-genesis.json" 2>/dev/null \
      || die "Rendered $out_dir/el-genesis.json is not valid JSON"
  fi

  if [[ -n "$prev_genesis" && "$prev_genesis" != "$(cat "$out_dir/el-genesis.json")" ]]; then
    warn "[$name] genesis changed since the last init — any existing chain data is now" \
         "incompatible. Run './setup.sh clean' before starting."
  fi

  log "[$name] chain-id $chain_id, validator $addr (${name}_MNEMONIC index $index)"

  # Spell out the alloc set. These are the only accounts that can pay for gas
  # on this chain, so a wrong or missing one is the first thing to check when a
  # tx fails with "insufficient funds".
  local i funded role
  log "[$name] genesis funds ${#CHAIN_ALLOC_ADDRS[@]} accounts with $(_genesis_balance_eth) each:"
  for (( i = 0; i < ${#CHAIN_ALLOC_ADDRS[@]}; i++ )); do
    funded="${CHAIN_ALLOC_ADDRS[$i]}"
    role=""
    [[ "$funded" == "${CHAIN_ACCT_ADDRS[0]}" ]] && role+=" [deployer]"
    [[ "$funded" == "$addr" ]] && role+=" [validator]"
    if (( i < ${#CHAIN_ACCT_ADDRS[@]} )); then
      log "  index $i  $funded${role}"
    else
      log "  index $index  $funded${role}"
    fi
  done

  _append_chain_env "$name" "$addr"
}

# Persist the derived values so `status`, later phases, and anything the user
# wants to source can read them without re-deriving. Written incrementally:
# every key is per chain now, and CHAIN_ACCT_* only holds the chain currently
# being initialised, so each chain appends its own block as it is rendered.
_start_chains_env() {
  # Truncated and locked down before a single byte goes in: _append_chain_env
  # writes each chain's deployer private key here, and the file would otherwise
  # be created under the caller's umask — 0644 by default. Unlike the validator
  # keys this one is only ever read by the host, so nothing needs it readable
  # beyond the owner.
  : > "$CHAINS_ENV_FILE"
  chmod 600 "$CHAINS_ENV_FILE"

  {
    echo "# Generated by ./setup.sh init — do not edit, do not commit."
    echo "#"
    echo "# DEMO KEYS — LOCAL DEVNET ONLY. The private keys below are derived from"
    echo "# publicly known BIP-39 test vectors and are worthless by design. Never"
    echo "# use them on a public, shared, or production network, never send real"
    echo "# funds to these addresses, and never copy them into another project."
    echo "# Kept at mode 0600 so the habit is right even though the keys are not."
    echo "#"
    echo "# Every account is per chain: A_* comes from A_MNEMONIC, B_* from B_MNEMONIC."
    echo "FUNDED_ACCOUNTS=$FUNDED_ACCOUNTS"
    echo "GENESIS_BALANCE=$GENESIS_BALANCE"
  } >> "$CHAINS_ENV_FILE"
}

# _append_chain_env <name> <validator-addr>
_append_chain_env() {
  local name="$1" validator="$2" i
  {
    echo
    echo "${name}_CHAIN_ID=$(_chain_attr "$name" CHAIN_ID)"
    echo "${name}_RPC_URL=http://localhost:$(_chain_attr "$name" RPC_PORT)"
    echo "${name}_RPC_URL_INTERNAL=http://$(_chain_attr "$name" SERVICE):8545"
    echo "${name}_VALIDATOR_ADDR=${validator}"
    echo "${name}_DEPLOYER_ADDR=${CHAIN_ACCT_ADDRS[0]}"
    echo "${name}_DEPLOYER_PRIVKEY=0x${CHAIN_ACCT_KEYS[0]}"
    # Both forms on purpose: the list is convenient to eyeball, the indexed vars
    # are safe to use from any shell (zsh does not word-split $A_FUNDED_ADDRS on
    # expansion the way bash does).
    echo "${name}_FUNDED_ADDRS=\"${CHAIN_ACCT_ADDRS[*]}\""
    for (( i = 0; i < ${#CHAIN_ACCT_ADDRS[@]}; i++ )); do
      echo "${name}_FUNDED_ADDR_${i}=${CHAIN_ACCT_ADDRS[$i]}"
    done
  } >> "$CHAINS_ENV_FILE"
}

init_chains() {
  mkdir -p "$LOCAL_DIR"
  chmod 755 "$LOCAL_DIR"
  _start_chains_env
  local name
  for name in "${CHAINS[@]}"; do
    _init_chain "$name"   # derives, renders, and appends its own chains.env block
  done
  log "Rendered chain configs into $LOCAL_DIR"
}

start_chains() {
  local name
  for name in "${CHAINS[@]}"; do
    [[ -f "$LOCAL_DIR/$name/key" ]] \
      || die "$LOCAL_DIR/$name/key is missing — run './setup.sh init' first"
  done
  log "Bringing up besu-a and besu-b..."
  docker compose up -d besu-a besu-b
}

wait_for_chains() {
  local name
  for name in "${CHAINS[@]}"; do
    wait_for_rpc "$(_chain_attr "$name" SERVICE)" \
                 "http://localhost:$(_chain_attr "$name" RPC_PORT)"
  done
}

print_accounts() {
  local name i index role
  for name in "${CHAINS[@]}"; do
    derive_chain_accounts "$name"
    index=$(_chain_attr "$name" VALIDATOR_INDEX)
    log "chain $name — $FUNDED_ACCOUNTS accounts from ${name}_MNEMONIC," \
        "each funded with $(_genesis_balance_eth) on $name only:"
    for (( i = 0; i < ${#CHAIN_ACCT_ADDRS[@]}; i++ )); do
      role=""
      [[ $i -eq 0 ]] && role+=" [deployer]"
      [[ $i -eq "$index" ]] && role+=" [validator]"
      log "  index $i  ${CHAIN_ACCT_ADDRS[$i]}${role}"
    done
  done
}

print_status() {
  local name block
  log "Chain status:"
  for name in "${CHAINS[@]}"; do
    block=$(eth_block_number "http://localhost:$(_chain_attr "$name" RPC_PORT)")
    log "  $(_chain_attr "$name" SERVICE) (chain-id $(_chain_attr "$name" CHAIN_ID))" \
        "RPC=http://localhost:$(_chain_attr "$name" RPC_PORT)" \
        "WS=ws://localhost:$(_chain_attr "$name" WS_PORT)" \
        "block=${block}"
  done
}

clean() {
  log "Stopping containers and removing volumes..."
  docker compose down -v --remove-orphans 2>/dev/null || true

  # Drop everything derived from the mnemonic so the next init starts from a
  # clean slate. The committed templates in chains/ are left alone.
  rm -rf "$LOCAL_DIR" 2>/dev/null || true

  log "Clean complete"
}
