package sandboxd

import (
	"encoding/json"
	"fmt"
	"os"
)

// activeStaticPrecompiles is the exact precompile set this chain registers (per the reference localnet
// recipe): only the ones the sandbox build wires up — no staking/distr/slashing/ics20/vesting. Enabling a
// precompile the chain does not register fails genesis validation, so this list is authoritative, not a
// superset.
var activeStaticPrecompiles = []any{
	"0x0000000000000000000000000000000000000100",
	"0x0000000000000000000000000000000000000400",
	"0x0000000000000000000000000000000000000804",
	"0x0000000000000000000000000000000000000805",
	"0x0000000000000000000000000000000000000807",
}

const (
	// blockMaxGas is the consensus block gas ceiling (default is "-1" = unlimited). The recipe pins it so
	// the EVM fixture deploys and relay txs have a defined, generous budget.
	blockMaxGas = "200000000"
	// poaValidatorPower is the POA voting power injected for the single genesis validator. POA has no
	// staking, so the number is arbitrary as long as it is the whole set's majority; the recipe uses 10000.
	poaValidatorPower = "10000"
	// ed25519PubKeyType is the proto type URL for a CometBFT ed25519 consensus key, the type `init`
	// generates in priv_validator_key.json (tendermint/PubKeyEd25519).
	ed25519PubKeyType = "/cosmos.crypto.ed25519.PubKey"
)

// genesisPatch carries the run-specific values the recipe injects into a freshly `init`ed genesis:
// the EVM/staking denom, the POA admin (also the IFT authority), and the consensus pubkey read back from
// the node's generated priv_validator_key.json so the single validator is actually in the set.
type genesisPatch struct {
	Denom               string // EVM + staking denom (astake)
	DisplayDenom        string // human display denom for bank metadata (stake)
	Admin               string // bech32 POA admin + IFT authority (the funded faucet account)
	Moniker             string // validator moniker (cosmetic)
	ValidatorPubKeyB64  string // base64 ed25519 consensus pubkey from priv_validator_key.json
	ValidatorPubKeyType string // proto type URL for the pubkey
}

// patchGenesis applies the localnet recipe to a genesis.json that `init` + `add-genesis-account` already
// wrote: it swaps the default "stake" denom to astake everywhere, installs the POA validator/admin, points
// the IFT authority and EVM denom, disables the base fee, whitelists the registered precompiles, writes the
// bank denom metadata, and lifts the block gas cap. It is a parse → mutate → write of the whole document
// (no jq/sed), so the transformation is one auditable Go step.
func patchGenesis(path string, p genesisPatch) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("sandboxd: read genesis %s: %w", path, err)
	}
	var root map[string]any
	if parseErr := json.Unmarshal(data, &root); parseErr != nil {
		return fmt.Errorf("sandboxd: parse genesis %s: %w", path, parseErr)
	}

	// 1. Replace the default "stake" denom everywhere (coins, gov deposits, evm_denom, ...). Whole-string
	// match only, so unrelated tokens like "stake-display" are untouched — the same effect as the recipe's
	// global denom substitution, done structurally.
	replaceDenom(root, "stake", p.Denom)

	appState, err := childMap(root, "app_state")
	if err != nil {
		return err
	}

	// 2. POA: admin + the single genesis validator (its consensus pubkey read from priv_validator_key.json,
	// so the node this genesis boots is actually in — and a supermajority of — the validator set).
	appState["poa"] = map[string]any{
		"params": map[string]any{"admin": p.Admin},
		"validators": []any{
			map[string]any{
				"pub_key": map[string]any{"@type": p.ValidatorPubKeyType, "key": p.ValidatorPubKeyB64},
				"power":   poaValidatorPower,
				"metadata": map[string]any{
					"moniker":          p.Moniker,
					"description":      "",
					"operator_address": p.Admin,
				},
			},
		},
		"allocated_fees": []any{},
	}

	// 3. IFT authority = the POA admin.
	if setErr := setIn(appState, p.Admin, "ift", "params", "authority"); setErr != nil {
		return setErr
	}

	// 4. EVM: denom + registered precompiles. (evm_denom is also caught by the denom swap above; set it
	// explicitly so the value never depends on the default having been literally "stake".)
	if setErr := setIn(appState, p.Denom, "evm", "params", "evm_denom"); setErr != nil {
		return setErr
	}
	if setErr := setIn(appState, activeStaticPrecompiles, "evm", "params", "active_static_precompiles"); setErr != nil {
		return setErr
	}

	// 5. Feemarket: no base fee — combined with a 0 min-gas-price this makes zero-gas-price txs the norm
	// (cosmos/evm eth_gasPrice returns 0x0), which is how the stub's transactor and the harness client send.
	if setErr := setIn(appState, true, "feemarket", "params", "no_base_fee"); setErr != nil {
		return setErr
	}

	// 6. Bank denom metadata for the 18-decimal astake unit (display "stake").
	denomMetadata := []any{
		map[string]any{
			"description": "The native token of the sandbox chain.",
			"denom_units": []any{
				map[string]any{"denom": p.Denom, "exponent": 0, "aliases": []any{"atto" + p.DisplayDenom}},
				map[string]any{"denom": p.DisplayDenom, "exponent": 18, "aliases": []any{}},
			},
			"base":     p.Denom,
			"display":  p.DisplayDenom,
			"name":     "Staking Token",
			"symbol":   "STAKE",
			"uri":      "",
			"uri_hash": "",
		},
	}
	if setErr := setIn(appState, denomMetadata, "bank", "denom_metadata"); setErr != nil {
		return setErr
	}

	// 7. Lift the consensus block gas cap from the default -1 (unlimited) to a defined ceiling.
	if setErr := setIn(root, blockMaxGas, "consensus", "params", "block", "max_gas"); setErr != nil {
		return setErr
	}

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("sandboxd: marshal patched genesis: %w", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("sandboxd: write patched genesis %s: %w", path, err)
	}
	return nil
}

// replaceDenom walks the decoded JSON and replaces every string value exactly equal to from with to.
func replaceDenom(v any, from, to string) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if s, ok := val.(string); ok {
				if s == from {
					t[k] = to
				}
				continue
			}
			replaceDenom(val, from, to)
		}
	case []any:
		for i, val := range t {
			if s, ok := val.(string); ok {
				if s == from {
					t[i] = to
				}
				continue
			}
			replaceDenom(val, from, to)
		}
	}
}

// childMap returns root[key] as a map, erroring if it is missing or the wrong shape — a malformed genesis
// (e.g. an upstream schema change) fails loudly here rather than producing a silently-wrong node.
func childMap(root map[string]any, key string) (map[string]any, error) {
	v, ok := root[key]
	if !ok {
		return nil, fmt.Errorf("sandboxd: genesis missing %q", key)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("sandboxd: genesis %q is not an object", key)
	}
	return m, nil
}

// setIn navigates root through the leading path segments (each must already be an object) and sets the
// final segment to value. A missing intermediate object is an error, so the patch never fabricates a
// section the chain does not define.
func setIn(root map[string]any, value any, path ...string) error {
	cur := root
	for _, seg := range path[:len(path)-1] {
		next, ok := cur[seg].(map[string]any)
		if !ok {
			return fmt.Errorf("sandboxd: genesis path %v: %q is missing or not an object", path, seg)
		}
		cur = next
	}
	cur[path[len(path)-1]] = value
	return nil
}
