// SPDX-License-Identifier: Apache-2.0

package lightclient

import (
	"context"
	"sort"

	"github.com/pkg/errors"
)

// ClientInfo describes a configured light client.
type ClientInfo struct {
	ClientID     string
	Type         string
	ClientParams *RawParams
}

// ChainInfo contains chain settings available to provers.
type ChainInfo struct {
	ChainID string
	EVM     *EVMChainInfo
}

// EVMChainInfo contains EVM chain settings.
type EVMChainInfo struct {
	RPC         string
	ICS26Router string
}

// ProverFactoryOptions contains inputs for constructing a prover.
type ProverFactoryOptions struct {
	Client            ClientInfo
	HostChain         ChainInfo
	CounterpartyChain ChainInfo
}

// ProverFactory builds provers for one custom light-client type.
type ProverFactory interface {
	// Type returns the configured client type name.
	Type() string

	// New builds a prover for Client.
	New(ctx context.Context, options ProverFactoryOptions) (Prover, error)
}

// Registry resolves custom light-client factories by type.
type Registry struct {
	factories map[string]ProverFactory
}

func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]ProverFactory)}
}

// Register adds a factory under its type name.
func (r *Registry) Register(f ProverFactory) error {
	if f == nil {
		return errors.New("factory must not be nil")
	}
	clientType := f.Type()
	switch clientType {
	case "":
		return errors.New("client type must not be empty")
	case "attestation":
		return errors.Errorf("client type %q is built in and cannot be overridden", clientType)
	}

	if _, exists := r.factories[clientType]; exists {
		return errors.Errorf("client type %q already registered", clientType)
	}

	r.factories[clientType] = f

	return nil
}

func (r *Registry) Get(clientType string) (ProverFactory, bool) {
	if r == nil {
		return nil, false
	}

	f, ok := r.factories[clientType]

	return f, ok
}

// Types returns the registered type names in sorted order.
func (r *Registry) Types() []string {
	if r == nil {
		return nil
	}

	types := make([]string, 0, len(r.factories))
	for name := range r.factories {
		types = append(types, name)
	}

	sort.Strings(types)

	return types
}
