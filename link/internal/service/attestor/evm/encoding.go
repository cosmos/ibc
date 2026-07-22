package evm

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

type payload struct {
	SourcePort string `abi:"sourcePort"`
	DestPort   string `abi:"destPort"`
	Version    string `abi:"version"`
	Encoding   string `abi:"encoding"`
	Value      []byte `abi:"value"`
}

type packet struct {
	Sequence         uint64    `abi:"sequence"`
	SourceClient     string    `abi:"sourceClient"`
	DestClient       string    `abi:"destClient"`
	TimeoutTimestamp uint64    `abi:"timeoutTimestamp"`
	Payloads         []payload `abi:"payloads"`
}

type stateAttestation struct {
	Height    uint64 `abi:"height"`
	Timestamp uint64 `abi:"timestamp"`
}

const (
	abiBytes  = "bytes"
	abiString = "string"
	abiTuple  = "tuple[]"
	abiUint64 = "uint64"
)

var (
	stateAttestationArgs = abi.Arguments{{Type: mustNewTuple(
		[]abi.ArgumentMarshaling{
			{Name: "height", Type: abiUint64},
			{Name: "timestamp", Type: abiUint64},
		},
	)}}

	packetArgs = abi.Arguments{{Type: mustNewTuple(
		[]abi.ArgumentMarshaling{
			{Name: "sequence", Type: abiUint64},
			{Name: "sourceClient", Type: abiString},
			{Name: "destClient", Type: abiString},
			{Name: "timeoutTimestamp", Type: abiUint64},
			{Name: "payloads", Type: abiTuple, Components: []abi.ArgumentMarshaling{
				{Name: "sourcePort", Type: abiString},
				{Name: "destPort", Type: abiString},
				{Name: "version", Type: abiString},
				{Name: "encoding", Type: abiString},
				{Name: "value", Type: abiBytes},
			}},
		},
	)}}
)

// EncodeStateAttestation encodes the Solidity StateAttestation struct.
func EncodeStateAttestation(height, timestamp uint64) ([]byte, error) {
	data, err := stateAttestationArgs.Pack(stateAttestation{
		Height:    height,
		Timestamp: timestamp,
	})
	if err != nil {
		return nil, fmt.Errorf("encode state attestation: %w", err)
	}

	return data, nil
}

// DecodePacket decodes a Solidity ABI-encoded IBC v2 packet.
func DecodePacket(data []byte) (v2.Packet, error) {
	values, err := packetArgs.Unpack(data)
	if err != nil {
		return v2.Packet{}, fmt.Errorf("unpack packet: %w", err)
	}

	var decoded struct {
		Packet packet
	}
	if err := packetArgs.Copy(&decoded, values); err != nil {
		return v2.Packet{}, fmt.Errorf("copy packet: %w", err)
	}

	payloads := make([]v2.Payload, len(decoded.Packet.Payloads))
	for i, item := range decoded.Packet.Payloads {
		payloads[i] = v2.Payload{
			SourcePort: item.SourcePort,
			DestPort:   item.DestPort,
			Version:    item.Version,
			Encoding:   item.Encoding,
			Value:      item.Value,
		}
	}

	return v2.Packet{
		Sequence:         decoded.Packet.Sequence,
		SourceClient:     decoded.Packet.SourceClient,
		DestClient:       decoded.Packet.DestClient,
		TimeoutTimestamp: decoded.Packet.TimeoutTimestamp,
		Payloads:         payloads,
	}, nil
}

func mustNewTuple(components []abi.ArgumentMarshaling) abi.Type {
	typ, err := abi.NewType("tuple", "", components)
	if err != nil {
		panic(err)
	}

	return typ
}
