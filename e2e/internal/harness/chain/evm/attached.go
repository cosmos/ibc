// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/ethclient"

	chainpkg "github.com/cosmos/ibc/e2e/internal/harness/chain"
)

type AttachedSpec struct {
	ID      string
	ChainID uint64
	RPCURL  string
}

type AttachedChain struct {
	*EVMClient
	Identity
}

var _ chainpkg.Chain = (*AttachedChain)(nil)

// The caller must close the returned connection.
func ConnectAttached(ctx context.Context, spec AttachedSpec) (*AttachedChain, error) {
	if spec.ID == "" {
		return nil, errors.New("attached chain id is empty")
	}
	if spec.ChainID == 0 {
		return nil, fmt.Errorf("attached chain %s: chain id is required to verify the node", spec.ID)
	}
	if spec.RPCURL == "" {
		return nil, fmt.Errorf("attached chain %s: rpc url is empty", spec.ID)
	}

	client, err := ethclient.DialContext(ctx, spec.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("dial attached chain %s at %s: %w", spec.ID, spec.RPCURL, err)
	}
	ec, err := NewVerifiedClient(ctx, client, spec.ChainID, fmt.Sprintf("attached chain %s", spec.ID))
	if err != nil {
		return nil, err
	}

	return &AttachedChain{EVMClient: ec, Identity: NewIdentity(spec.ID, spec.RPCURL)}, nil
}
