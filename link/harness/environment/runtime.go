package environment

import (
	"fmt"

	"github.com/cosmos/ibc/link/harness/chain/evm"
)

// Runtime contains process-local bindings that must not be serialized with a Spec.
// Endpoint URLs and private keys may contain credentials, so neither is copied into
// the environment manifest.
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
	snapshot := Runtime{
		Endpoints:   make(map[EndpointBindingID]EndpointBinding, len(r.Endpoints)),
		Authorities: make(map[AuthorityID]EVMAuthority, len(r.Authorities)),
	}
	for id, binding := range r.Endpoints {
		snapshot.Endpoints[id] = binding
	}
	for id, binding := range r.Authorities {
		snapshot.Authorities[id] = binding
	}
	return snapshot
}
