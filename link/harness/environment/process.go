package environment

import "github.com/cosmos/ibc/link/harness/ibclink"

// BindIBCLink gives a Driver process-local access to every resolved Chain
// endpoint without exposing endpoint values to the caller. The binding is
// non-owning and becomes unusable when the Environment closes.
func (e *Environment) BindIBCLink(driver *ibclink.Driver) error {
	bind := func() error {
		resolvers := make(map[string]func() (string, error), len(e.chains))
		for id, resolved := range e.chains {
			chain := resolved
			resolvers[string(id)] = func() (string, error) {
				return chain.rpcURL, nil
			}
		}
		acquire := func() (func(), error) {
			if e.lease == nil {
				return func() {}, nil
			}
			return e.lease.acquire()
		}
		return driver.BindChainRPCs(resolvers, acquire)
	}
	if e.lease == nil {
		return bind()
	}
	return e.lease.use(bind)
}
