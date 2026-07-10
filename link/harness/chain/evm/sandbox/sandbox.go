package sandbox

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/cosmos/ibc/link/harness/chain/evm"
	"github.com/cosmos/ibc/link/harness/chain/sandboxd"

	sdkbech32 "github.com/cosmos/cosmos-sdk/types/bech32"
	chainpkg "github.com/cosmos/ibc/link/harness/chain"
)

// Spec configures one logical EVM chain backed by a managed sandboxd (Cosmos SDK + cosmos/evm) node.
type Spec struct {
	ID      string // logical chain id used across the harness
	ChainID uint64 // EVM numeric chain id (must be unique across the topology)
	WorkDir string // per-chain working dir; the node home and files live under it
	LogPath string // combined node output log (empty: discard)
	// RelayerKeyHex is the hex secp256k1 relayer signing key (the wire chain's EVMSignerKey). Its derived
	// address is genesis-funded (bech32-encoded) so the relayer can pay gas for its destination effects.
	RelayerKeyHex string
}

// Chain is a real Cosmos SDK "sandboxd" node the harness owns, presented to the harness as an
// EVM chain: cosmos/evm exposes a full eth JSON-RPC over the same account/state model, so the embedded
// EVMClient — the identical dial/fund/broadcast/query path Anvil and Besu use — drives it unchanged. The
// dev faucet works here because Start genesis-funds the faucet's (and relayer's) address bytes,
// bech32-encoded under the "cosmos" prefix; cosmos/evm then surfaces that balance at the same 20-byte hex
// address the eth client signs from.
//
// It implements neither chain.BlockController nor chain.FaultInjector, both named gaps:
//   - BlockController (pause/mine/advance-time): a real CometBFT node produces blocks on its own consensus
//     timer; there is no on-demand-mining or time-warp cheat like Anvil's, so tests needing those gate on
//     the instant-anvil lane (RequireAnvilLane) and skip here.
//   - FaultInjector (stop/restart the node mid-test): a Cosmos node persists its state to the home dir and
//     recovers it on restart, so a restart-preserves-state story is genuinely possible — but it is future
//     work (the harness would need to re-derive the same dynamic ports and rebind), so it is not advertised
//     now. A test asking for either capability gets the harness's standard ErrCapabilityMissing error.
type Chain struct {
	*evm.EVMClient
	evm.Identity

	node *sandboxd.Node
}

var (
	_ chainpkg.Chain            = (*Chain)(nil)
	_ chainpkg.ReceiverProvider = (*Chain)(nil)
	_ evm.ClientProvider        = (*Chain)(nil)
)

// Start launches a sandboxd node (init → fund → patch → start → semantic readiness, all in the
// sandboxd package), dials its eth JSON-RPC, verifies the reported chain id matches spec, and wraps the
// connection in an EVMClient bound to the shared dev faucet. ctx governs startup only; the node stays alive
// until Stop.
func Start(ctx context.Context, spec Spec) (*Chain, error) {
	if spec.ID == "" {
		return nil, fmt.Errorf("sandbox chain id is empty")
	}
	if spec.ChainID == 0 {
		return nil, fmt.Errorf("sandbox chain %s: EVM chain id is required", spec.ID)
	}
	if spec.WorkDir == "" {
		return nil, fmt.Errorf("sandbox chain %s: work dir is empty", spec.ID)
	}
	if spec.RelayerKeyHex == "" {
		return nil, fmt.Errorf("sandbox chain %s: relayer key is empty", spec.ID)
	}

	// Genesis-fund the two dev identities the stub and harness sign with (faucet: deploy/mint/transfers and
	// funding fresh accounts; relayer: destination-side effects). Both need a large native balance because
	// NewFundedAccount hands out 100 ETH at a time; the genesis default (10 tokens) would run dry. The
	// relayer's address is derived from the config-declared signer key, not a shared constant.
	relayer, err := evm.AccountFromHex(spec.RelayerKeyHex)
	if err != nil {
		return nil, fmt.Errorf("sandbox chain %s: relayer key: %w", spec.ID, err)
	}
	faucetBech32, err := sandboxBech32(evm.FaucetAccount().Addr)
	if err != nil {
		return nil, fmt.Errorf("sandbox chain %s: encode faucet address: %w", spec.ID, err)
	}
	relayerBech32, err := sandboxBech32(relayer.Addr)
	if err != nil {
		return nil, fmt.Errorf("sandbox chain %s: encode relayer address: %w", spec.ID, err)
	}
	coins := sandboxPrefundCoins()

	node, err := sandboxd.StartNode(ctx, sandboxd.Spec{
		ID:         spec.ID,
		ChainID:    fmt.Sprintf("sandbox-%d-1", spec.ChainID),
		EVMChainID: spec.ChainID,
		HomeDir:    filepath.Join(spec.WorkDir, "home"),
		LogPath:    spec.LogPath,
		Admin:      faucetBech32,
		Genesis: []sandboxd.GenesisAccount{
			{Address: faucetBech32, Coins: coins},
			{Address: relayerBech32, Coins: coins},
		},
	})
	if err != nil {
		return nil, err
	}

	ok := false
	defer func() {
		if !ok {
			_ = node.Stop()
		}
	}()

	client, err := ethclient.DialContext(ctx, node.JSONRPCURL())
	if err != nil {
		return nil, fmt.Errorf("sandbox chain %s: dial %s: %w", spec.ID, node.JSONRPCURL(), err)
	}
	ec, err := evm.NewVerifiedClient(ctx, client, spec.ChainID, fmt.Sprintf("sandbox chain %s", spec.ID))
	if err != nil {
		return nil, err
	}

	ok = true
	return &Chain{EVMClient: ec, Identity: evm.NewIdentity(spec.ID, node.JSONRPCURL()), node: node}, nil
}

// LogPath is the file capturing combined node output (named to avoid clashing with the embedded
// EVMClient.Logs, which returns on-chain event logs).
func (c *Chain) LogPath() string { return c.node.LogPath() }

// Stop closes the eth client and gracefully terminates the node.
func (c *Chain) Stop() error {
	c.Close()
	return c.node.Stop()
}

// sandboxBech32 encodes a dev account's 20 address bytes under sandboxd's account prefix — the form
// genesis funding must use for cosmos/evm to surface the balance at the same 20-byte eth address.
func sandboxBech32(addr common.Address) (string, error) {
	return sdkbech32.ConvertAndEncode(sandboxd.Bech32HRP, addr.Bytes())
}

// sandboxPrefundCoins is the genesis balance for each funded dev account, in astake (1:1 with wei).
func sandboxPrefundCoins() string {
	return chainpkg.GenesisPrefund().String() + sandboxd.Denom
}
