// SPDX-License-Identifier: Apache-2.0

package loadtest

import (
	"testing"

	"github.com/skip-mev/catalyst/ift/accounts"
	"github.com/stretchr/testify/require"
)

func TestSpecIFTWallets(t *testing.T) {
	// ARRANGE
	const mnemonic = "rotate stumble once topic possible message powder recall turkey legend depart brick"
	spec, err := NewSpecIFT(
		3, 50, 50, mnemonic,
		EVMEndpoint{ChainID: "31337", RPC: "http://127.0.0.1:8545", Websocket: "ws://127.0.0.1:8546"},
		IFTToken{ClientID: "client-0", Address: "0x0000000000000000000000000000000000000001"},
	)
	require.NoError(t, err)

	// ACT
	first, err := spec.Wallets()
	require.NoError(t, err)
	second, err := spec.Wallets()

	// ASSERT
	require.NoError(t, err)
	require.Len(t, first, 3)
	require.Equal(t, first, second)
	for i, got := range first {
		want, err := accounts.EVMAddressFromMnemonic(mnemonic, i)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
	require.NotEqual(t, first[0], first[1])
}
