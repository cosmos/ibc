// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/attestation"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/besuqbft"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/ibcmulticall"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/ics26router"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/ift"
)

type revertError struct {
	code int
	data any
}

func (e revertError) Error() string  { return "execution reverted" }
func (e revertError) ErrorCode() int { return e.code }
func (e revertError) ErrorData() any { return e.data }

// revertData is the hex revert data of the named custom error.
func revertData(t testing.TB, metadata *bind.MetaData, name string, args ...any) string {
	t.Helper()
	contractABI, err := metadata.GetAbi()
	require.NoError(t, err)
	customErr, ok := contractABI.Errors[name]
	require.True(t, ok)
	packed, err := customErr.Inputs.Pack(args...)
	require.NoError(t, err)
	return hexutil.Encode(append(customErr.ID[:4], packed...))
}

// namedReverts are custom errors ExplainRevert names.
var namedReverts = []struct {
	metadata *bind.MetaData
	name     string
	args     []any
	want     string
}{
	{ics26router.ContractMetaData, "IBCClientNotFound", []any{"client-0"}, "IBCClientNotFound(client-0)"},
	{attestation.ContractMetaData, "ConsensusTimestampNotFound", []any{uint64(7)}, "ConsensusTimestampNotFound(7)"},
	{ift.ContractMetaData, "RateLimitExceeded", nil, "RateLimitExceeded()"},
	{ibcmulticall.ContractMetaData, "IBCMulticallCallOutOfGas", []any{big.NewInt(2)}, "IBCMulticallCallOutOfGas(2)"},
	{
		besuqbft.ContractMetaData, "ConsensusStateExpired",
		[]any{uint64(100), big.NewInt(121), uint64(20)},
		"ConsensusStateExpired(100, 121, 20)",
	},
}

func TestExplainRevert(t *testing.T) {
	for _, tc := range namedReverts {
		t.Run("names "+tc.name, func(t *testing.T) {
			data := revertData(t, tc.metadata, tc.name, tc.args...)
			require.Equal(t, tc.want, ExplainRevertData(hexutil.MustDecode(data)))
			// geth and Besu 26.x use code 3, older Besu -32000
			for _, code := range []int{3, -32000} {
				original := revertError{code: code, data: data}
				err := ExplainRevert(original)
				require.EqualError(t, err, tc.want+": execution reverted")
				require.ErrorIs(t, err, original)
			}
		})
	}

	t.Run("keeps unknown data", func(t *testing.T) {
		err := ExplainRevert(revertError{data: "0xdeadbeef"})
		require.EqualError(t, err, "0xdeadbeef: execution reverted")
	})

	t.Run("keeps errors without data", func(t *testing.T) {
		plain := errors.New("connection refused")
		require.Equal(t, plain, ExplainRevert(plain))
	})
}

func TestExplainRevertData(t *testing.T) {
	stringType, err := abi.NewType("string", "", nil)
	require.NoError(t, err)
	message, err := (abi.Arguments{{Type: stringType}}).Pack("rate limit exceeded")
	require.NoError(t, err)
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"empty", nil, "empty revert data"},
		{"short", []byte{1, 2}, "0x0102"},
		{"unknown", hexutil.MustDecode("0xdeadbeef"), "0xdeadbeef"},
		{"revert string", append(hexutil.MustDecode("0x08c379a0"), message...), "rate limit exceeded"},
		{"malformed revert", hexutil.MustDecode("0x08c379a0"), "0x08c379a0"},
		{"panic", hexutil.MustDecode("0x4e487b710000000000000000000000000000000000000000000000000000000000000011"), "arithmetic underflow or overflow"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, ExplainRevertData(tc.data))
		})
	}
}
