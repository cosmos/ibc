package external

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/cosmos/ibc/link/harness/chain/evm"

	chainpkg "github.com/cosmos/ibc/link/harness/chain"
)

// Spec identifies an already-running EVM chain the harness connects to but does not own.
type Spec struct {
	ID      string // logical chain id used across the harness
	ChainID uint64 // expected EVM numeric chain id; Connect verifies the node reports it
	RPCURL  string // host-reachable RPC URL (already resolved; no ${ENV} for the POC harness dial)
}

// Chain is an EVM chain running out-of-band that the harness connects to but does not own. It
// exposes its EVM view via the embedded EVMClient, but deliberately implements neither
// chain.BlockController nor chain.FaultInjector: the harness cannot pause mining, advance time, or
// stop/restart a node it did not launch. A test that asks for one of those capabilities gets the harness's
// standard ErrCapabilityMissing error — the same negotiation as any other missing capability.
//
// POC funding gap (named, not a fallback): the embedded EVMClient funds fresh accounts from the shared dev
// faucet (testkeys.FaucetPrivateKeyHex). That only works here because the proof test points
// Connect at an out-of-band Anvil, whose default genesis prefunds that key. A real external chain
// — a public testnet, a partner's node — would have no such balance: a production external-chain story
// needs a real funding source wired in at this boundary. The harness deliberately does not paper over it.
type Chain struct {
	*evm.EVMClient
	evm.Identity
}

var (
	_ chainpkg.Chain            = (*Chain)(nil)
	_ chainpkg.ReceiverProvider = (*Chain)(nil)
	_ evm.ClientProvider        = (*Chain)(nil)
)

// Connect dials the external chain's RPC, verifies the reported chain id matches spec (the same
// check anvil.Start performs on a launched node), and wraps the connection in an EVMClient bound to the
// shared dev faucet. ctx governs the dial and verification only; the connection stays open until Close.
func Connect(ctx context.Context, spec Spec) (*Chain, error) {
	if spec.ID == "" {
		return nil, errors.New("external chain id is empty")
	}
	if spec.ChainID == 0 {
		return nil, fmt.Errorf("external chain %s: chain id is required to verify the node", spec.ID)
	}
	if spec.RPCURL == "" {
		return nil, fmt.Errorf("external chain %s: rpc url is empty", spec.ID)
	}

	client, err := ethclient.DialContext(ctx, spec.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("dial external chain %s at %s: %w", spec.ID, spec.RPCURL, err)
	}
	ec, err := evm.NewVerifiedClient(ctx, client, spec.ChainID, fmt.Sprintf("external chain %s", spec.ID))
	if err != nil {
		return nil, err
	}

	return &Chain{EVMClient: ec, Identity: evm.NewIdentity(spec.ID, spec.RPCURL)}, nil
}
