// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func TestReceiptEvents(t *testing.T) {
	contract := common.HexToAddress("0x1")
	wanted := common.HexToHash("0x1")
	decodeErr := errors.New("decode event")
	parse := func(log types.Log) (*byte, error) {
		if len(log.Topics) == 0 {
			return nil, bind.ErrNoEventSignature
		}
		if log.Topics[0] != wanted {
			return nil, bind.ErrEventSignatureMismatch
		}
		if len(log.Data) == 0 {
			return nil, decodeErr
		}
		return &log.Data[0], nil
	}

	events, err := receiptEvents(&types.Receipt{Logs: []*types.Log{
		{Address: common.HexToAddress("0x2"), Topics: []common.Hash{wanted}},
		{Address: contract},
		{Address: contract, Topics: []common.Hash{common.HexToHash("0x2")}},
		{Address: contract, Topics: []common.Hash{wanted}, Data: []byte{7}},
		{Address: contract, Topics: []common.Hash{wanted}, Data: []byte{8}},
	}}, contract, parse)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, byte(7), *events[0])
	require.Equal(t, byte(8), *events[1])

	_, err = receiptEvents(&types.Receipt{Logs: []*types.Log{
		{Address: contract, Topics: []common.Hash{wanted}},
	}}, contract, parse)
	require.ErrorIs(t, err, decodeErr)
}
