// Package ibc translates IBC packets to and from the Solidity router ABI.
package ibc

import (
	"fmt"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/accounts/abi"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
)

var packetArgs = mustPacketArgs()

// EncodePacket encodes an IBC packet as the Solidity router tuple.
func EncodePacket(p channeltypesv2.Packet) ([]byte, error) {
	payloads := make([]ics26router.IICS26RouterMsgsPayload, len(p.Payloads))
	for i, item := range p.Payloads {
		payloads[i] = ics26router.IICS26RouterMsgsPayload{
			SourcePort: item.SourcePort,
			DestPort:   item.DestinationPort,
			Version:    item.Version,
			Encoding:   item.Encoding,
			Value:      item.Value,
		}
	}

	data, err := packetArgs.Pack(ics26router.IICS26RouterMsgsPacket{
		Sequence:         p.Sequence,
		SourceClient:     p.SourceClient,
		DestClient:       p.DestinationClient,
		TimeoutTimestamp: p.TimeoutTimestamp,
		Payloads:         payloads,
	})
	if err != nil {
		return nil, fmt.Errorf("encode packet: %w", err)
	}

	return data, nil
}

// DecodePacket decodes a Solidity router tuple into an IBC packet.
func DecodePacket(data []byte) (channeltypesv2.Packet, error) {
	values, err := packetArgs.Unpack(data)
	if err != nil {
		return channeltypesv2.Packet{}, fmt.Errorf("unpack packet: %w", err)
	}

	var decoded struct {
		Packet ics26router.IICS26RouterMsgsPacket
	}
	if err := packetArgs.Copy(&decoded, values); err != nil {
		return channeltypesv2.Packet{}, fmt.Errorf("copy packet: %w", err)
	}

	payloads := make([]channeltypesv2.Payload, len(decoded.Packet.Payloads))
	for i, item := range decoded.Packet.Payloads {
		payloads[i] = channeltypesv2.Payload{
			SourcePort:      item.SourcePort,
			DestinationPort: item.DestPort,
			Version:         item.Version,
			Encoding:        item.Encoding,
			Value:           item.Value,
		}
	}

	return channeltypesv2.Packet{
		Sequence:          decoded.Packet.Sequence,
		SourceClient:      decoded.Packet.SourceClient,
		DestinationClient: decoded.Packet.DestClient,
		TimeoutTimestamp:  decoded.Packet.TimeoutTimestamp,
		Payloads:          payloads,
	}, nil
}

func mustPacketArgs() abi.Arguments {
	contractABI, err := ics26router.ContractMetaData.GetAbi()
	if err != nil {
		panic(err)
	}

	return contractABI.Events["SendPacket"].Inputs.NonIndexed()
}
