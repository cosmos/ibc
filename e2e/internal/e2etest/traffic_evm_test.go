// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/ics26router"
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

func TestSentPackets(t *testing.T) {
	router := common.HexToAddress("0x1")
	sendPacket := mustABI(ics26router.ContractMetaData).Events["SendPacket"]
	sendPacketLog := func(address common.Address, packet ics26router.IICS26RouterMsgsPacket) *types.Log {
		data, err := sendPacket.Inputs.NonIndexed().Pack(packet)
		require.NoError(t, err)
		return &types.Log{
			Address: address,
			Topics: []common.Hash{
				sendPacket.ID,
				crypto.Keccak256Hash([]byte(packet.SourceClient)),
				common.BigToHash(new(big.Int).SetUint64(packet.Sequence)),
			},
			Data: data,
		}
	}
	packet := func(client string, sequence uint64) ics26router.IICS26RouterMsgsPacket {
		return ics26router.IICS26RouterMsgsPacket{
			Sequence:     sequence,
			SourceClient: client,
			DestClient:   "dest",
			Payloads: []ics26router.IICS26RouterMsgsPayload{{
				SourcePort: "transfer",
				DestPort:   "transfer",
				Version:    "ics20-1",
				Encoding:   "application/x-solidity-abi",
				Value:      []byte{1},
			}},
		}
	}

	receipt := &types.Receipt{Logs: []*types.Log{
		sendPacketLog(router, packet("client-0", 1)),
		sendPacketLog(router, packet("client-1", 1)),
		sendPacketLog(common.HexToAddress("0x2"), packet("client-0", 2)),
		sendPacketLog(router, packet("client-0", 3)),
	}}

	got, err := SentPackets(router, "client-0", receipt)
	require.NoError(t, err)
	require.Equal(t, []ics26router.IICS26RouterMsgsPacket{packet("client-0", 1), packet("client-0", 3)}, got)

	got, err = SentPackets(router, "client-2", receipt)
	require.NoError(t, err)
	require.Empty(t, got)
}
