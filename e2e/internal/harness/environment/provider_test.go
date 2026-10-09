// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManagedEVMChains(t *testing.T) {
	anvil, err := ManagedEVMChains(EVMProviderAnvil, "a", "b")
	require.NoError(t, err)
	require.Equal(t, []ChainSpec{
		ManagedAnvil{ID: "a", EVMChainID: 31337},
		ManagedAnvil{ID: "b", EVMChainID: 31338},
	}, anvil)

	besu, err := ManagedEVMChains(EVMProviderBesu, "a", "b")
	require.NoError(t, err)
	require.Equal(t, []ChainSpec{
		ManagedBesu{ID: "a", EVMChainID: 32337},
		ManagedBesu{ID: "b", EVMChainID: 32338},
	}, besu)

	_, err = ManagedEVMChains("other", "a")
	require.Error(t, err)
}

func TestParseEVMProvider(t *testing.T) {
	for _, value := range []string{"anvil", "besu"} {
		got, err := ParseEVMProvider(value)
		require.NoError(t, err)
		require.Equal(t, EVMProvider(value), got)
	}
	_, err := ParseEVMProvider("geth")
	require.Error(t, err)
}
