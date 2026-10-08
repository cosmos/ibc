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

func TestTransferBatchValidatesBeforeSending(t *testing.T) {
	recipient := common.HexToAddress("0x0000000000000000000000000000000000000001")
	for _, tc := range []struct {
		name     string
		requests []ERCTransferRequest
		wantErr  string
	}{
		{
			name:     "nil amount",
			requests: []ERCTransferRequest{{Address: recipient}},
			wantErr:  "ERC transfer 0: traffic: Transfer amount is required",
		},
		{
			name:     "non-positive amount",
			requests: []ERCTransferRequest{{Address: recipient, Amount: big.NewInt(1)}, {Address: recipient, Amount: big.NewInt(0)}},
			wantErr:  "ERC transfer 1: traffic: Transfer amount must be positive",
		},
		{
			name:     "zero address",
			requests: []ERCTransferRequest{{Amount: big.NewInt(1)}},
			wantErr:  "ERC transfer 0 has a zero address",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A zero IFT has no Chain: any attempt to send would panic, so an
			// error here proves validation runs before the first transaction.
			receipts, err := (&IFT{routeID: "route"}).TransferBatch(t.Context(), tc.requests)
			require.ErrorContains(t, err, tc.wantErr)
			require.Nil(t, receipts)
		})
	}
}

func TestTransferBatchEmptyIsNoop(t *testing.T) {
	receipts, err := (&IFT{routeID: "route"}).TransferBatch(t.Context(), nil)
	require.NoError(t, err)
	require.Empty(t, receipts)
}
