// SPDX-License-Identifier: Apache-2.0

package proofgen

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

	// New builds a generator for self, which tracks counterparty. Params have
	// already passed ValidateParams.
	New(ctx context.Context, deps Deps, self, counterparty ClientEnd) (ProofGenerator, error)
}

// Registry resolves a Factory by client type name. The relayer registers its
// built-in types; callers add their own before starting a relayer.
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

// Merge registers every entry of other into r, failing on any name r already
// has. Used to fold caller-supplied custom types into the relayer's built-ins.
func (r *Registry) Merge(other *Registry) error {
	if other == nil {
		return nil
	}

	for name, f := range other.factories {
		if err := r.Register(name, f); err != nil {
			return err
		}
	}

	return nil
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
