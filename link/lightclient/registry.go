// SPDX-License-Identifier: Apache-2.0

package lightclient

import (
	"context"
	"sort"

	"github.com/pkg/errors"
)

// ProverFactory builds proof generators for one custom light-client type.
// New validates the client parameters as part of construction.
type ProverFactory interface {
	// Type returns the name operators use in connection configuration.
	Type() string

	// New builds a generator for Client, which tracks CounterpartyChain.
	New(ctx context.Context, options ProverFactoryOptions) (Prover, error)
}

// Registry resolves custom light-client factories by config type name.
// Built-in client types are resolved internally by the relayer.
//
// A Registry is not safe for concurrent registration. Build it fully during
// startup, then treat it as read-only.
type Registry struct {
	factories map[string]ProverFactory
}

func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]ProverFactory)}
}

// Register associates a factory with the client type returned by Type.
// Registering a type twice is an error rather than a silent overwrite.
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

// Types lists every registered client type, sorted. Useful for error messages.
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
