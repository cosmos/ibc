// SPDX-License-Identifier: Apache-2.0

package lightclient

import (
	"context"
	"sort"

	"github.com/pkg/errors"
)

// Factory builds proof generators for one light client type and validates that
// type's config params.
type Factory interface {
	// ValidateParams reports whether params are usable for this client type.
	// It runs during config validation, before any chain access, so it must
	// not perform I/O. Decode through RawParams.Decode so unknown fields are
	// rejected.
	ValidateParams(params *RawParams) error

	// New builds a generator for Client, which tracks Counterparty. ClientParams
	// have already passed ValidateParams.
	New(ctx context.Context, options FactoryOptions) (ProofGenerator, error)
}

// Registry resolves custom light-client factories by config type name.
// Built-in client types are resolved internally by the relayer.
//
// A Registry is not safe for concurrent registration. Build it fully during
// startup, then treat it as read-only.
type Registry struct {
	factories map[string]Factory
}

func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

// Register associates a client type name with a factory. The name is the
// string operators write as `type:` in their connection config. Registering a
// name twice is an error rather than a silent overwrite.
func (r *Registry) Register(clientType string, f Factory) error {
	switch {
	case clientType == "":
		return errors.New("client type must not be empty")
	case f == nil:
		return errors.Errorf("client type %q: factory must not be nil", clientType)
	}

	if _, exists := r.factories[clientType]; exists {
		return errors.Errorf("client type %q already registered", clientType)
	}

	r.factories[clientType] = f

	return nil
}

func (r *Registry) Get(clientType string) (Factory, bool) {
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
