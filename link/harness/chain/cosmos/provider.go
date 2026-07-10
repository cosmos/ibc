package cosmos

import "github.com/cosmos/ibc/link/harness/chain"

// ClientProvider is the capability a cosmos-family chain advertises to expose its concrete read client —
// the cosmos analog of evm.ClientProvider. Provider chains satisfy it by returning their owned Client
// (CometBFT tx_search + gRPC bank queries). It is deliberately a single method: cosmos operations live on
// the concrete client type, so this interface cannot grow into a second client surface.
type ClientProvider interface {
	Cosmos() *Client
}

// FromChain resolves c's concrete cosmos read client, reporting false when c is not a cosmos-family chain
// (or withholds the capability). It is how the composition root binds a cosmos reader without naming any
// concrete chain type, so a second cosmos provider only has to advertise ClientProvider.
func FromChain(c chain.Chain) (*Client, bool) {
	p, ok := chain.As[ClientProvider](c)
	if !ok {
		return nil, false
	}
	return p.Cosmos(), true
}
