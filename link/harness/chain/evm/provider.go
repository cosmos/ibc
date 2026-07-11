package evm

import "github.com/cosmos/ibc/link/harness/chain"

type Identity struct {
	id     string
	rpcURL string
}

func NewIdentity(id, rpcURL string) Identity { return Identity{id: id, rpcURL: rpcURL} }

func (i Identity) ID() string           { return i.id }
func (i Identity) Family() chain.Family { return chain.FamilyEVM }
func (i Identity) RPCURL() string       { return i.rpcURL }

type ClientProvider interface {
	EVM() *EVMClient
}

func FromChain(c chain.Chain) (*EVMClient, bool) {
	p, ok := chain.As[ClientProvider](c)
	if !ok {
		return nil, false
	}
	return p.EVM(), true
}
