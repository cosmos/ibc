package reader

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/onchain"
)

func TestDecodeIFTMintReceived(t *testing.T) {
	receiver := common.HexToAddress("0x66ab6d9362d4f35596279692f0251db635165871")
	event := mockIFTABI.Events[eventIFTMintReceived]
	data, err := event.Inputs.NonIndexed().Pack("client-a", big.NewInt(42))
	require.NoError(t, err)

	got, err := decodeIFTMintReceived(types.Log{
		Topics: []common.Hash{event.ID, common.BytesToHash(receiver.Bytes())},
		Data:   data,
	})
	require.NoError(t, err)
	require.Equal(t, "client-a", got.ClientID)
	require.Equal(t, receiver, got.Receiver)
	require.Equal(t, int64(42), got.Amount.Int64())
}

func TestIFTMintReceivedMatchesUpstreamABI(t *testing.T) {
	event := mockIFTABI.Events[eventIFTMintReceived]
	require.Equal(t, crypto.Keccak256Hash([]byte("IFTMintReceived(string,address,uint256)")), event.ID)
	require.Len(t, event.Inputs, 3)
	require.Equal(t, "clientId", event.Inputs[0].Name)
	require.False(t, event.Inputs[0].Indexed)
	require.Equal(t, "receiver", event.Inputs[1].Name)
	require.True(t, event.Inputs[1].Indexed)
	require.Equal(t, "amount", event.Inputs[2].Name)
	require.False(t, event.Inputs[2].Indexed)
}

func TestAwaitIFTReceivedRequiresDeploymentClientID(t *testing.T) {
	r := New(nil, "chain-a", wire.ChainDeployment{}, onchain.Budget{})
	_, err := r.AwaitIFTReceived(context.Background(), "route-a", 1)
	require.ErrorContains(t, err, "deployment has no IBC client id")
}

// canonicalAddrReader builds a reader without a live client — CanonicalAddr is pure string handling.
func canonicalAddrReader() onchain.Reader {
	return New(nil, "chain-a", wire.ChainDeployment{}, onchain.Budget{})
}

func TestCanonicalAddrFoldsCasing(t *testing.T) {
	r := canonicalAddrReader()

	lower, err := r.CanonicalAddr("0x66ab6d9362d4f35596279692f0251db635165871")
	require.NoError(t, err)
	upper, err := r.CanonicalAddr("0x66AB6D9362D4F35596279692F0251DB635165871")
	require.NoError(t, err)
	require.Equal(t, lower, upper, "casing variants of one address share a canonical form")
	require.Equal(t, "0x66aB6D9362d4F35596279692F0251Db635165871", lower, "canonical form is EIP-55")
}

func TestCanonicalAddrRejectsMalformed(t *testing.T) {
	r := canonicalAddrReader()
	for _, s := range []string{"", "0x123", "not-an-address", "cosmos1qqqsyqcyq5rqwzqfpg9scrgwpugpzysn7ykx8h"} {
		_, err := r.CanonicalAddr(s)
		require.Error(t, err, "input %q", s)
		require.ErrorContains(t, err, "not a valid EVM address")
	}
}
