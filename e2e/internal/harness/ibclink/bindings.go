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

const chainRPCEnvPrefix = "IBC_LINK_CHAIN_RPC_"

type chainRPCBinding struct {
	envName string
	resolve func() (string, error)
}

// processBindingSet pairs endpoint resolvers with the lifecycle borrow that
// keeps their resources alive. Installed once during setup, immutable after.
type processBindingSet struct {
	chainRPC     map[string]chainRPCBinding
	acquireLease func() (release func(), err error)
}

// BindChainRPCs binds resolved Chain endpoints without exposing their values.
// Bindings can be installed only once.
func (r *Driver) BindChainRPCs(
	resolvers map[string]func() (string, error),
	acquire func() (release func(), err error),
) error {
	if acquire == nil {
		return errors.New("ibclink: process binding lease is required")
	}
	chainRPC := make(map[string]chainRPCBinding, len(resolvers))
	for chainID, resolve := range resolvers {
		if chainID == "" {
			return errors.New("ibclink: cannot bind an empty Chain id")
		}
		if resolve == nil {
			return fmt.Errorf("ibclink: Chain %q RPC resolver is required", chainID)
		}
		chainRPC[chainID] = chainRPCBinding{
			envName: chainRPCEnvName(chainID),
			resolve: resolve,
		}
	}

	if r.bindings != nil {
		return errors.New("ibclink: Chain RPC bindings are already installed")
	}
	r.bindings = &processBindingSet{chainRPC: chainRPC, acquireLease: acquire}
	return nil
}

// ChainRPC returns the configuration reference for a bound Chain.
func (r *Driver) ChainRPC(chainID string) (string, error) {
	if r.bindings == nil {
		return "", fmt.Errorf("ibclink: no RPC binding for Chain %q", chainID)
	}
	binding, ok := r.bindings.chainRPC[chainID]
	if !ok {
		return "", fmt.Errorf("ibclink: no RPC binding for Chain %q", chainID)
	}
	return "${" + binding.envName + "}", nil
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
	if len(b.chainRPC) == 0 {
		return nil, nil
	}

	chainIDs := make([]string, 0, len(b.chainRPC))
	for chainID := range b.chainRPC {
		chainIDs = append(chainIDs, chainID)
	}
	slices.Sort(chainIDs)

	env := os.Environ()
	for _, chainID := range chainIDs {
		binding := b.chainRPC[chainID]
		value, err := resolveChainRPC(chainID, binding)
		if err != nil {
			return nil, err
		}
		env = append(env, binding.envName+"="+value)
	}
	return env, nil
}

func resolveChainRPC(chainID string, binding chainRPCBinding) (string, error) {
	value, err := binding.resolve()
	if err != nil {
		return "", fmt.Errorf("ibclink: resolve RPC binding for Chain %q: %w", chainID, err)
	}
	if value == "" {
		return "", fmt.Errorf("ibclink: RPC binding for Chain %q is empty", chainID)
	}
	return value, nil
}

func chainRPCEnvName(chainID string) string {
	return chainRPCEnvPrefix + strings.ToUpper(hex.EncodeToString([]byte(chainID)))
}
