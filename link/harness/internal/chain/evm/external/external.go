package external

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/cosmos/ibc/link/harness/chain/evm"

	chainpkg "github.com/cosmos/ibc/link/harness/chain"
)

type Spec struct {
	ID      string
	ChainID uint64
	RPCURL  string
}

type Chain struct {
	*evm.EVMClient
	evm.Identity
}

var (
	_ chainpkg.Chain            = (*Chain)(nil)
	_ chainpkg.ReceiverProvider = (*Chain)(nil)
	_ evm.ClientProvider        = (*Chain)(nil)
)

// The caller must close the returned connection.
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
