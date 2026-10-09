// SPDX-License-Identifier: Apache-2.0

package traffic

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestCoins(t *testing.T) {
	want, ok := new(big.Int).SetString("3000000000000000000", 10)
	require.True(t, ok)
	require.Equal(t, want, Coins(3))
	require.Zero(t, Coins(0).Sign())
}

func TestFundWalletsValidatesBeforeSending(t *testing.T) {
	wallet := common.HexToAddress("0x1")
	tests := []struct {
		name      string
		wallets   []common.Address
		tokenEach *big.Int
		weiEach   *big.Int
		want      string
	}{
		{
			name:    "missing token amount",
			wallets: []common.Address{wallet},
			weiEach: big.NewInt(1),
			want:    "amount is required",
		},
		{
			name:      "zero token amount",
			wallets:   []common.Address{wallet},
			tokenEach: big.NewInt(0),
			weiEach:   big.NewInt(1),
			want:      "must be positive",
		},
		{name: "missing wei", wallets: []common.Address{wallet}, tokenEach: big.NewInt(1), want: "non-negative"},
		{
			name:      "negative wei",
			wallets:   []common.Address{wallet},
			tokenEach: big.NewInt(1),
			weiEach:   big.NewInt(-1),
			want:      "non-negative",
		},
		{
			name:      "zero address",
			wallets:   []common.Address{wallet, {}},
			tokenEach: big.NewInt(1),
			weiEach:   big.NewInt(1),
			want:      "wallet 1 has a zero address",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := (&IFT{routeID: "route"}).FundWallets(t.Context(), tc.wallets, tc.tokenEach, tc.weiEach)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestFundWalletsEmptyIsNoop(t *testing.T) {
	require.NoError(t, (&IFT{routeID: "route"}).FundWallets(t.Context(), nil, big.NewInt(1), big.NewInt(0)))
}
