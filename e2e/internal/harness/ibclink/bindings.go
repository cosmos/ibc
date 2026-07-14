package ibclink

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/cosmos/ibc/link/cmd/configcmd"
)

const chainRPCEnvPrefix = "IBC_LINK_CHAIN_RPC_"

type chainRPCBinding struct {
	envName string
	resolve func() (string, error)
}

type processBindingSet struct {
	chainRPC     map[string]chainRPCBinding
	acquireLease func() (release func(), err error)
}

// processBindings publishes one immutable binding set. Taking one snapshot
// keeps its endpoint resolvers and lifecycle borrow inseparable.
type processBindings struct {
	mu  sync.RWMutex
	set *processBindingSet
}

type processEnvironment struct {
	variables []string
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

	r.bindings.mu.Lock()
	defer r.bindings.mu.Unlock()
	if r.bindings.set != nil {
		return errors.New("ibclink: Chain RPC bindings are already installed")
	}
	r.bindings.set = &processBindingSet{chainRPC: chainRPC, acquireLease: acquire}
	return nil
}

// ChainRPC returns the configuration reference for a bound Chain.
func (r *Driver) ChainRPC(chainID string) (configcmd.RPC, error) {
	bindings := r.bindings.snapshot()
	if bindings == nil {
		return configcmd.RPC{}, fmt.Errorf("ibclink: no RPC binding for Chain %q", chainID)
	}
	binding, ok := bindings.chainRPC[chainID]
	if !ok {
		return configcmd.RPC{}, fmt.Errorf("ibclink: no RPC binding for Chain %q", chainID)
	}
	return configcmd.RPC{URL: "${" + binding.envName + "}"}, nil
}

func (r *Driver) withProcessEnv(use func(processEnvironment) error) error {
	env, release, err := r.acquireProcessEnv()
	if err != nil {
		return err
	}
	defer release()
	return use(env)
}

func (r *Driver) acquireProcessEnv() (processEnvironment, func(), error) {
	bindings := r.bindings.snapshot()
	release, err := bindings.acquire()
	if err != nil {
		return processEnvironment{}, nil, err
	}
	env, err := bindings.resolveProcessEnv()
	if err != nil {
		release()
		return processEnvironment{}, nil, err
	}
	return env, release, nil
}

func (r *processBindings) snapshot() *processBindingSet {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.set
}

func (b *processBindingSet) acquire() (func(), error) {
	if b == nil {
		return func() {}, nil
	}
	release, err := b.acquireLease()
	if err != nil {
		return nil, err
	}
	if release == nil {
		return nil, errors.New("ibclink: process binding lease returned no release function")
	}
	return release, nil
}

// resolveProcessEnv runs while the binding lease is held. Environment-backed
// resolvers rely on that borrow to keep their resources alive.
func (b *processBindingSet) resolveProcessEnv() (processEnvironment, error) {
	if b == nil || len(b.chainRPC) == 0 {
		return processEnvironment{}, nil
	}

	chainIDs := make([]string, 0, len(b.chainRPC))
	for chainID := range b.chainRPC {
		chainIDs = append(chainIDs, chainID)
	}
	slices.Sort(chainIDs)

	env := processEnvironment{variables: os.Environ()}
	for _, chainID := range chainIDs {
		binding := b.chainRPC[chainID]
		value, err := resolveChainRPC(chainID, binding)
		if err != nil {
			return processEnvironment{}, err
		}
		env.variables = append(env.variables, binding.envName+"="+value)
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
