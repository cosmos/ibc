// SPDX-License-Identifier: Apache-2.0

package traffic

import (
	"errors"
	"math/big"
	"testing"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	attestoribc "github.com/cosmos/ibc/cli/attestor/evm/ibc"
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

func TestSentPacketsOnlyDecodesRouterLogs(t *testing.T) {
	routerABI := mustABI(ics26router.ContractMetaData)
	event := routerABI.Events["SendPacket"]
	sendPacketLog := func(address common.Address, sequence uint64) *types.Log {
		packet := ics26router.IICS26RouterMsgsPacket{
			Sequence:         sequence,
			SourceClient:     "client-0",
			DestClient:       "client-1",
			TimeoutTimestamp: 100,
			Payloads:         []ics26router.IICS26RouterMsgsPayload{},
		}
		data, err := event.Inputs.NonIndexed().Pack(packet)
		require.NoError(t, err)
		return &types.Log{
			Address: address,
			Topics: []common.Hash{
				event.ID,
				crypto.Keccak256Hash([]byte(packet.SourceClient)),
				common.BigToHash(new(big.Int).SetUint64(sequence)),
			},
			Data: data,
		}
	}
	router := common.HexToAddress("0x1")

	events, err := SentPackets(&types.Receipt{Logs: []*types.Log{
		sendPacketLog(common.HexToAddress("0x2"), 1),
		sendPacketLog(router, 2),
		{Address: router, Topics: []common.Hash{common.HexToHash("0x3")}},
		sendPacketLog(router, 3),
	}}, router)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, uint64(2), events[0].Packet.Sequence)
	require.Equal(t, uint64(3), events[1].Packet.Sequence)
	require.Equal(t, "client-1", events[1].Packet.DestClient)
	attested, err := attestoribc.DecodePacket(events[1].Raw.Data)
	require.NoError(t, err, "the log data is the encoding attestors attest")
	require.Equal(t, uint64(3), attested.Sequence)
}
