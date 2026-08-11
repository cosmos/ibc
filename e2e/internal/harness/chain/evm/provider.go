// SPDX-License-Identifier: Apache-2.0

package evm

import "github.com/cosmos/ibc/e2e/internal/harness/chain"

type Identity struct {
	id     string
	rpcURL string
}

func NewIdentity(id, rpcURL string) Identity { return Identity{id: id, rpcURL: rpcURL} }

func (i Identity) ID() string     { return i.id }
func (i Identity) RPCURL() string { return i.rpcURL }

type clientAccessor interface {
	WithEVMClient(func(*EVMClient) error) error
}

// WithChainClient resolves the provider's current client for each operation.
func WithChainClient(c chain.Chain, use func(*EVMClient) error) (bool, error) {
	if accessor, ok := c.(clientAccessor); ok {
		return true, accessor.WithEVMClient(use)
	}
	return false, nil
}
