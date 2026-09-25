// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"errors"
	"math/big"
	"testing"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/attestation"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besuqbft"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

type revertError struct {
	code int
	data any
}

func (e revertError) Error() string  { return "execution reverted" }
func (e revertError) ErrorCode() int { return e.code }
func (e revertError) ErrorData() any { return e.data }

func TestExplainRevert(t *testing.T) {
	for _, tc := range []struct {
		metadata *bind.MetaData
		name     string
		args     []any
		want     string
	}{
		{ics26router.ContractMetaData, "IBCClientNotFound", []any{"client-0"}, "IBCClientNotFound(client-0)"},
		{attestation.ContractMetaData, "ConsensusTimestampNotFound", []any{uint64(7)}, "ConsensusTimestampNotFound(7)"},
		{
			besuqbft.ContractMetaData, "ConsensusStateExpired",
			[]any{uint64(100), big.NewInt(121), uint64(20)},
			"ConsensusStateExpired(100, 121, 20)",
		},
	} {
		t.Run("names "+tc.name, func(t *testing.T) {
			contractABI, err := tc.metadata.GetAbi()
			require.NoError(t, err)
			customErr, ok := contractABI.Errors[tc.name]
			require.True(t, ok)
			args, err := customErr.Inputs.Pack(tc.args...)
			require.NoError(t, err)

			data := hexutil.Encode(append(customErr.ID[:4], args...))
			// geth and Besu 26.x use code 3, older Besu -32000
			for _, code := range []int{3, -32000} {
				err = explainRevert(revertError{code: code, data: data})
				require.EqualError(t, err, tc.want+": execution reverted")
			}
		})
	}

	t.Run("keeps unknown data", func(t *testing.T) {
		err := explainRevert(revertError{data: "0xdeadbeef"})
		require.EqualError(t, err, "execution reverted")
	})

	t.Run("keeps errors without data", func(t *testing.T) {
		plain := errors.New("connection refused")
		require.Equal(t, plain, explainRevert(plain))
	})
}
