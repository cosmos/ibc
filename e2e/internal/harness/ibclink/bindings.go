// SPDX-License-Identifier: Apache-2.0

package ibclink

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
)

const (
	chainRPCEnvPrefix = "IBC_LINK_CHAIN_RPC_"
	chainWSEnvPrefix  = "IBC_LINK_CHAIN_WS_"
)

// ChainEndpoints are one Chain's relayer-facing endpoints. WS is empty for
// Chains that serve no websocket.
type ChainEndpoints struct {
	RPC string
	WS  string
}

type chainEndpointBinding struct {
	rpcEnvName string
	wsEnvName  string
	resolve    func() (ChainEndpoints, error)
}

// processBindingSet pairs endpoint resolvers with the lifecycle borrow that
// keeps their resources alive. Installed once during setup, immutable after.
type processBindingSet struct {
	chains       map[string]chainEndpointBinding
	acquireLease func() (release func(), err error)
}

// BindChainEndpoints binds resolved Chain endpoints without exposing their
// values. Bindings can be installed only once.
func (r *Driver) BindChainEndpoints(
	resolvers map[string]func() (ChainEndpoints, error),
	acquire func() (release func(), err error),
) error {
	if acquire == nil {
		return errors.New("ibclink: process binding lease is required")
	}
	chains := make(map[string]chainEndpointBinding, len(resolvers))
	for chainID, resolve := range resolvers {
		if chainID == "" {
			return errors.New("ibclink: cannot bind an empty Chain id")
		}
		if resolve == nil {
			return fmt.Errorf("ibclink: Chain %q endpoint resolver is required", chainID)
		}
		chains[chainID] = chainEndpointBinding{
			rpcEnvName: chainRPCEnvName(chainID),
			wsEnvName:  chainWSEnvName(chainID),
			resolve:    resolve,
		}
	}

	if r.bindings != nil {
		return errors.New("ibclink: Chain RPC bindings are already installed")
	}
	r.bindings = &processBindingSet{chains: chains, acquireLease: acquire}
	return nil
}

// ChainRPC returns the configuration reference for a bound Chain.
func (r *Driver) ChainRPC(chainID string) (string, error) {
	binding, err := r.chainBinding(chainID)
	if err != nil {
		return "", err
	}
	return "${" + binding.rpcEnvName + "}", nil
}

// ChainWS returns the configuration reference for a bound Chain's websocket
// endpoint. It expands to the empty string when the Chain serves none.
func (r *Driver) ChainWS(chainID string) (string, error) {
	binding, err := r.chainBinding(chainID)
	if err != nil {
		return "", err
	}
	return "${" + binding.wsEnvName + "}", nil
}

func (r *Driver) chainBinding(chainID string) (chainEndpointBinding, error) {
	if r.bindings == nil {
		return chainEndpointBinding{}, fmt.Errorf("ibclink: no RPC binding for Chain %q", chainID)
	}
	binding, ok := r.bindings.chains[chainID]
	if !ok {
		return chainEndpointBinding{}, fmt.Errorf("ibclink: no RPC binding for Chain %q", chainID)
	}
	return binding, nil
}

// acquireProcessEnv resolves the child-process environment while borrowing
// the binding lease; the caller must invoke release when the process no
// longer needs the bound resources.
func (r *Driver) acquireProcessEnv() ([]string, func(), error) {
	if r.bindings == nil {
		return nil, func() {}, nil
	}
	release, err := r.bindings.acquireLease()
	if err != nil {
		return nil, nil, err
	}
	if release == nil {
		return nil, nil, errors.New("ibclink: process binding lease returned no release function")
	}
	env, err := r.bindings.resolveProcessEnv()
	if err != nil {
		release()
		return nil, nil, err
	}
	return env, release, nil
}

// resolveProcessEnv runs while the binding lease is held. Environment-backed
// resolvers rely on that borrow to keep their resources alive. A nil result
// makes exec inherit the parent environment.
func (b *processBindingSet) resolveProcessEnv() ([]string, error) {
	if len(b.chains) == 0 {
		return nil, nil
	}

	chainIDs := make([]string, 0, len(b.chains))
	for chainID := range b.chains {
		chainIDs = append(chainIDs, chainID)
	}
	slices.Sort(chainIDs)

	env := os.Environ()
	for _, chainID := range chainIDs {
		binding := b.chains[chainID]
		endpoints, err := resolveChainEndpoints(chainID, binding)
		if err != nil {
			return nil, err
		}
		env = append(env, binding.rpcEnvName+"="+endpoints.RPC, binding.wsEnvName+"="+endpoints.WS)
	}
	return env, nil
}

func resolveChainEndpoints(chainID string, binding chainEndpointBinding) (ChainEndpoints, error) {
	endpoints, err := binding.resolve()
	if err != nil {
		return ChainEndpoints{}, fmt.Errorf("ibclink: resolve RPC binding for Chain %q: %w", chainID, err)
	}
	if endpoints.RPC == "" {
		return ChainEndpoints{}, fmt.Errorf("ibclink: RPC binding for Chain %q is empty", chainID)
	}
	return endpoints, nil
}

func chainRPCEnvName(chainID string) string {
	return chainRPCEnvPrefix + strings.ToUpper(hex.EncodeToString([]byte(chainID)))
}

func chainWSEnvName(chainID string) string {
	return chainWSEnvPrefix + strings.ToUpper(hex.EncodeToString([]byte(chainID)))
}
