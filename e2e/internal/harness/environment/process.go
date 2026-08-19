// SPDX-License-Identifier: Apache-2.0

package environment

import "github.com/cosmos/ibc/e2e/internal/harness/ibclink"

// BindIBCLink gives a Driver process-local access to every resolved Chain
// endpoint without exposing endpoint values to the caller. The binding is
// non-owning and becomes unusable when the Environment closes.
func (e *Environment) BindIBCLink(driver *ibclink.Driver) error {
	return e.lease.use(func() error {
		resolvers := make(map[string]func() (ibclink.ChainEndpoints, error), len(e.chains))
		for id, resolved := range e.chains {
			resolvers[string(id)] = func() (ibclink.ChainEndpoints, error) {
				return ibclink.ChainEndpoints{RPC: resolved.rpcURL, WS: resolved.wsURL}, nil
			}
		}
		return driver.BindChainEndpoints(resolvers, e.lease.acquire)
	})
}
