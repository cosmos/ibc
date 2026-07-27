package environment

import (
	"fmt"
	"maps"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
)

// Runtime contains process-local bindings that are separate from the durable Spec.
type Runtime struct {
	Endpoints   map[EndpointBindingID]EndpointBinding
	Authorities map[AuthorityID]EVMAuthority
}

type EndpointBinding struct {
	RPCURL string
}

type EVMAuthority struct {
	PrivateKeyHex string
}

func (r Runtime) endpoint(id EndpointBindingID) (EndpointBinding, bool) {
	b, ok := r.Endpoints[id]
	return b, ok
}

func (r Runtime) authority(id AuthorityID) (EVMAuthority, bool) {
	b, ok := r.Authorities[id]
	return b, ok
}

func (r Runtime) evmAccount(id AuthorityID) (evm.Account, error) {
	binding, ok := r.authority(id)
	if !ok || binding.PrivateKeyHex == "" {
		return evm.Account{}, fmt.Errorf("environment: no runtime authority binding for %q", id)
	}
	account, err := evm.AccountFromHex(binding.PrivateKeyHex)
	if err != nil {
		return evm.Account{}, fmt.Errorf("environment: invalid runtime authority binding for %q: %w", id, err)
	}
	return account, nil
}

func (r Runtime) snapshot() Runtime {
	return Runtime{
		Endpoints:   maps.Clone(r.Endpoints),
		Authorities: maps.Clone(r.Authorities),
	}
}
