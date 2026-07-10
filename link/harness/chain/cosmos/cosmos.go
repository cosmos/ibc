// Package cosmos is the harness-side cosmos facet of the sandboxd node: it drives the same sandboxd binary
// as harness/chain/evm/sandbox, but through its cosmos surfaces (CometBFT RPC + bank module + bech32
// accounts), not its cosmos/evm JSON-RPC. It is the second chain family the harness owns: a genuine Cosmos
// SDK chain with its own address form, signing, and native IFT/ICS-27-GMP applications.
//
// The package owns the CosmosChain lifecycle and the harness read client. The stub has its own cosmos client;
// both sides agree only on public chain surfaces and shared literals.
package cosmos

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/cosmos/ibc/link/harness/chain"
	"github.com/cosmos/ibc/link/harness/chain/sandboxd"

	sdkbech32 "github.com/cosmos/cosmos-sdk/types/bech32"
)

const (
	// GMPDenom is the bank denom the cosmos GMP "counter" stands in as — the cosmos analog of the solidity
	// Counter fixture (counter-as-balance). The harness genesis-mints it to the relayer, the stub executor
	// sends 1 unit relayer->target per increment, and the reader reads the target's balance of it as the count.
	// A shared contract with the stub's own copy; both sides pin the same literal.
	GMPDenom = "ugmpc"

	// GMPIncrementPayload is the fixed GMP call payload that means "increment the counter" — the cosmos
	// analog of Counter.increment() calldata. The harness reader hands this exact payload as the default
	// GMP payload; the stub executor increments only when the delivered payload equals it. Shared
	// contract with the stub's own copy.
	GMPIncrementPayload = "increment"

	// Bech32HRP is the account-address prefix. The 20-byte account bytes re-encode to cosmos1... under it.
	Bech32HRP = sandboxd.Bech32HRP

	// nodeEVMChainID is the EVM chain id the underlying sandboxd node is initialized with. The cosmos facet
	// never dials the node's eth JSON-RPC, so the value is arbitrary (each node is an isolated subprocess
	// with its own dynamic ports); it only has to be non-zero to satisfy sandboxd's --evm.evm-chain-id.
	nodeEVMChainID = 262144
)

// Spec configures one cosmos-facet sandboxd node.
type Spec struct {
	ID            string // logical chain id used across the harness
	CosmosChainID string // the CometBFT chain-id string (from wire.Chain.CosmosChainID); the stub signs with it
	SignerKeyHex  string // the relayer/admin signer, plain-secp256k1 hex (from wire.Chain.SignerKey)
	FaucetKeyHex  string // the Cosmos user/faucet, plain-secp256k1 hex (from wire.Chain.FaucetKey)
	WorkDir       string // per-chain working dir; the node home and files live under it
	LogPath       string // combined node output log (empty: discard)
}

// CosmosChain is a real Cosmos SDK sandboxd node the harness owns, presented as a cosmos-family chain: it
// implements chain.Chain (Family cosmos, RPCURL = the CometBFT RPC — the endpoint the ibc link config points
// at) with no EVM view (it advertises no evm.ClientProvider), and chain.ReceiverProvider (see NewReceiver —
// the non-EVM difference the seam exists to prove). It advertises neither BlockController nor FaultInjector, the same
// named gaps as the EVM sandbox facet (a CometBFT node has no on-demand-mining or time-warp cheat).
//
//nolint:revive // Deliberate stutter: the family prefix keeps call sites greppable (cosmos.CosmosChain).
type CosmosChain struct {
	id     string
	node   *sandboxd.Node
	client *Client
}

var (
	_ chain.Chain            = (*CosmosChain)(nil)
	_ chain.ReceiverProvider = (*CosmosChain)(nil)
	_ chain.GRPCProvider     = (*CosmosChain)(nil)
	_ ClientProvider         = (*CosmosChain)(nil)
)

// StartCosmos launches a sandboxd node driven as a Cosmos chain: it derives the relayer/admin bech32 from
// the signer key, genesis-funds the relayer and user, starts the node with its gRPC query server enabled,
// and returns once the node is semantically ready
// (blocks committing AND the gRPC server serving — both asserted in sandboxd.StartNode). ctx governs startup
// only; the node stays alive until Stop.
func StartCosmos(ctx context.Context, spec Spec) (*CosmosChain, error) {
	if spec.ID == "" {
		return nil, fmt.Errorf("cosmos chain id is empty")
	}
	if spec.CosmosChainID == "" {
		return nil, fmt.Errorf("cosmos chain %s: cosmos chain-id is empty", spec.ID)
	}
	if spec.SignerKeyHex == "" {
		return nil, fmt.Errorf("cosmos chain %s: signer key is empty", spec.ID)
	}
	if spec.FaucetKeyHex == "" {
		return nil, fmt.Errorf("cosmos chain %s: faucet key is empty", spec.ID)
	}
	if spec.WorkDir == "" {
		return nil, fmt.Errorf("cosmos chain %s: work dir is empty", spec.ID)
	}

	relayer, err := AccountAddressFromKeyHex(spec.SignerKeyHex)
	if err != nil {
		return nil, fmt.Errorf("cosmos chain %s: derive relayer address: %w", spec.ID, err)
	}
	// The user/faucet is a second funded account, distinct from the relayer/admin. Deployment mints the native
	// tokenfactory IFT supply to it after startup.
	faucet, err := AccountAddressFromKeyHex(spec.FaucetKeyHex)
	if err != nil {
		return nil, fmt.Errorf("cosmos chain %s: derive faucet address: %w", spec.ID, err)
	}

	node, err := sandboxd.StartNode(ctx, sandboxd.Spec{
		ID:         spec.ID,
		ChainID:    spec.CosmosChainID,
		EVMChainID: nodeEVMChainID,
		HomeDir:    filepath.Join(spec.WorkDir, "home"),
		LogPath:    spec.LogPath,
		Admin:      relayer, // POA admin + IFT authority
		Genesis: []sandboxd.GenesisAccount{
			{Address: relayer, Coins: prefundCoins()},
			{Address: faucet, Coins: prefundCoins()},
		},
		EnableGRPC: true, // the cosmos facet reads over typed gRPC (bank/auth queries)
	})
	if err != nil {
		return nil, err
	}

	// The chain owns its client for its whole lifetime (dialed here, closed in Stop), exactly as an EVM
	// chain owns its EVMClient; the family's reader borrows it for reads and the AppSubmitter for the
	// source user submissions. It signs source submissions with the user/faucet key (the relayer/admin
	// key is the SUT's), matching the distinct EVM source faucet.
	client, err := NewClient(node.CometRPCURL(), node.GRPCURL(), spec.CosmosChainID, spec.FaucetKeyHex)
	if err != nil {
		_ = node.Stop()
		return nil, fmt.Errorf("cosmos chain %s: %w", spec.ID, err)
	}

	return &CosmosChain{
		id:     spec.ID,
		node:   node,
		client: client,
	}, nil
}

func (c *CosmosChain) ID() string           { return c.id }
func (c *CosmosChain) Family() chain.Family { return chain.FamilyCosmos }

// RPCURL is the CometBFT RPC endpoint — the URL the compiled ibc link config points the stub at for this
// chain (the stub's cosmos client broadcasts and polls there), and the endpoint the harness reader's
// tx_search reads. gRPC is the second read surface, carried separately.
func (c *CosmosChain) RPCURL() string { return c.node.CometRPCURL() }

// GRPCURL is the cosmos gRPC dial target (host:port) — the second read surface (bank/auth queries),
// advertised through the chain.GRPCProvider capability so the runtime bindings can project it into the
// ibc link config's per-chain gRPC URL.
func (c *CosmosChain) GRPCURL() string { return c.node.GRPCURL() }

// Cosmos is the ClientProvider capability hook: the chain's read client (CometBFT tx_search + gRPC bank
// queries), owned by the chain and borrowed by the family's reader. It lives until Stop.
func (c *CosmosChain) Cosmos() *Client { return c.client }

// Height reports the latest committed block height, via the chain-owned read client.
func (c *CosmosChain) Height(ctx context.Context) (uint64, error) { return c.client.Height(ctx) }

// LogPath is the file capturing combined node output.
func (c *CosmosChain) LogPath() string { return c.node.LogPath() }

// NewReceiver satisfies chain.ReceiverProvider: it mints a fresh cosmos-native receiver as 20 random bytes
// bech32-encoded under the "cosmos" prefix. A cosmos bank MsgSend creates the recipient account on first
// receipt, so a receiver needs neither a signing key nor a prior
// balance. That absence (versus an EVM receiver, which is a funded account) is exactly the family
// difference the ReceiverProvider seam exists to prove.
func (c *CosmosChain) NewReceiver(_ context.Context) (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("cosmos: generate receiver: %w", err)
	}
	return sdkbech32.ConvertAndEncode(Bech32HRP, buf)
}

// Stop releases the chain's read client and gracefully terminates the node.
func (c *CosmosChain) Stop() error { return errors.Join(c.client.Close(), c.node.Stop()) }

// prefundCoins is the genesis balance string for a funded account: a huge amount of astake (the staking/fee
// denom) and ugmpc (the GMP counter stand-in). The native IFT tokenfactory denom is created and minted by
// deployment after the chain starts. Coins are comma-separated and denom-sorted for genesis validation.
func prefundCoins() string {
	amt := chain.GenesisPrefund().String()
	return amt + sandboxd.Denom + "," + amt + GMPDenom
}
