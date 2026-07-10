package evm

import "github.com/cosmos/ibc/link/harness/chain"

// Identity is the id + host-reachable RPC URL every EVM-family provider Chain exposes identically. Embed it
// (constructed via NewIdentity) to inherit ID()/Family()/RPCURL(); Family is always FamilyEVM.
type Identity struct {
	id     string
	rpcURL string
}

// NewIdentity builds the shared EVM-family Chain identity from a logical id and RPC URL.
func NewIdentity(id, rpcURL string) Identity { return Identity{id: id, rpcURL: rpcURL} }

func (i Identity) ID() string           { return i.id }
func (i Identity) Family() chain.Family { return chain.FamilyEVM }
func (i Identity) RPCURL() string       { return i.rpcURL }

// ClientProvider is the capability an EVM-family chain advertises to expose its concrete client — the
// whole EVM view of a chain.Chain in one accessor. Provider chains satisfy it by embedding *EVMClient
// (whose EVM method returns itself). It is deliberately a single method: EVM operations live on the
// concrete client type, so this interface cannot grow into a second client surface.
type ClientProvider interface {
	EVM() *EVMClient
}

// FromChain resolves c's concrete EVM client, reporting false when c is not an EVM-family chain.
func FromChain(c chain.Chain) (*EVMClient, bool) {
	p, ok := chain.As[ClientProvider](c)
	if !ok {
		return nil, false
	}
	return p.EVM(), true
}
