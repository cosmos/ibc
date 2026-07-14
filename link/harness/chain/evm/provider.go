package evm

import "github.com/cosmos/ibc/link/harness/chain"

type Identity struct {
	id     string
	rpcURL string
}

func NewIdentity(id, rpcURL string) Identity { return Identity{id: id, rpcURL: rpcURL} }

func (i Identity) ID() string     { return i.id }
func (i Identity) RPCURL() string { return i.rpcURL }

type clientProvider interface {
	EVM() *EVMClient
}

// clientAccessor lets adapters protect a replaceable client for the duration
// of an operation. Providers with a stable client can rely on clientProvider.
type clientAccessor interface {
	WithEVMClient(func(*EVMClient) error) error
}

func SupportsClientAccess(c chain.Chain) bool {
	if _, ok := c.(clientAccessor); ok {
		return true
	}
	_, ok := c.(clientProvider)
	return ok
}

func fromChain(c chain.Chain) (*EVMClient, bool) {
	p, ok := c.(clientProvider)
	if !ok {
		return nil, false
	}
	return p.EVM(), true
}

// WithChainClient resolves the provider's current client for each operation.
func WithChainClient(c chain.Chain, use func(*EVMClient) error) (bool, error) {
	if accessor, ok := c.(clientAccessor); ok {
		return true, accessor.WithEVMClient(use)
	}
	client, ok := fromChain(c)
	if !ok {
		return false, nil
	}
	return true, use(client)
}
