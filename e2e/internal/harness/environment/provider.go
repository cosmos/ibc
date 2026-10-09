// SPDX-License-Identifier: Apache-2.0

package environment

import "fmt"

// EVMProvider selects the managed EVM node implementation.
type EVMProvider string

const (
	EVMProviderAnvil EVMProvider = "anvil"
	EVMProviderBesu  EVMProvider = "besu"
)

// EVM chain id bases of managed Chains: the i-th Chain gets base+i.
const (
	anvilChainIDBase = 31337
	besuChainIDBase  = 32337
)

// ParseEVMProvider validates a provider name.
func ParseEVMProvider(value string) (EVMProvider, error) {
	switch provider := EVMProvider(value); provider {
	case EVMProviderAnvil, EVMProviderBesu:
		return provider, nil
	default:
		return "", fmt.Errorf("unknown EVM provider %q: want anvil or besu", value)
	}
}

// ManagedEVMChains declares one managed Chain per id on provider, with EVM
// chain ids counting up from the provider's base.
func ManagedEVMChains(provider EVMProvider, ids ...ChainID) ([]ChainSpec, error) {
	chains := make([]ChainSpec, len(ids))
	for i, id := range ids {
		switch provider {
		case EVMProviderAnvil:
			chains[i] = ManagedAnvil{ID: id, EVMChainID: anvilChainIDBase + uint64(i)}
		case EVMProviderBesu:
			chains[i] = ManagedBesu{ID: id, EVMChainID: besuChainIDBase + uint64(i)}
		default:
			return nil, fmt.Errorf("unknown EVM provider %q: want anvil or besu", provider)
		}
	}
	return chains, nil
}
