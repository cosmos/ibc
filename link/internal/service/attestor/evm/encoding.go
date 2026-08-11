// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
)

// PacketCompact mirrors the Solidity PacketCompact struct.
type PacketCompact struct {
	Path       [32]byte `abi:"path"`
	Commitment [32]byte `abi:"commitment"`
}

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

type packetAttestation struct {
	Height  uint64          `abi:"height"`
	Packets []PacketCompact `abi:"packets"`
}

// AttestationProof mirrors IAttestationMsgs.AttestationProof
type AttestationProof struct {
	AttestationData []byte   `abi:"attestationData"`
	Signatures      [][]byte `abi:"signatures"`
}

const (
	abiBytes      = "bytes"
	abiBytes32    = "bytes32"
	abiBytesSlice = "bytes[]"
	abiString     = "string"
	abiTuple      = "tuple[]"
	abiUint64     = "uint64"
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

	packetAttestationArgs = abi.Arguments{{Type: mustNewTuple(
		[]abi.ArgumentMarshaling{
			{Name: "height", Type: abiUint64},
			{Name: "packets", Type: abiTuple, Components: []abi.ArgumentMarshaling{
				{Name: "path", Type: abiBytes32},
				{Name: "commitment", Type: abiBytes32},
			}},
		},
	)}}

	attestationProofArgs = abi.Arguments{{Type: mustNewTuple(
		[]abi.ArgumentMarshaling{
			{Name: "attestationData", Type: abiBytes},
			{Name: "signatures", Type: abiBytesSlice},
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

// DecodeStateAttestation decodes a Solidity ABI-encoded StateAttestation struct
func DecodeStateAttestation(data []byte) (height, timestamp uint64, err error) {
	values, err := stateAttestationArgs.Unpack(data)
	if err != nil {
		return 0, 0, fmt.Errorf("unpack state attestation: %w", err)
	}

	var decoded struct {
		StateAttestation stateAttestation
	}
	if err := stateAttestationArgs.Copy(&decoded, values); err != nil {
		return 0, 0, fmt.Errorf("copy state attestation: %w", err)
	}

	return decoded.StateAttestation.Height, decoded.StateAttestation.Timestamp, nil
}

// EncodePacketAttestation encodes the Solidity PacketAttestation struct.
func EncodePacketAttestation(height uint64, packets []PacketCompact) ([]byte, error) {
	data, err := packetAttestationArgs.Pack(packetAttestation{
		Height:  height,
		Packets: packets,
	})
	if err != nil {
		return nil, fmt.Errorf("encode packet attestation: %w", err)
	}

	return data, nil
}

// DecodePacketAttestation decodes a Solidity ABI-encoded PacketAttestation struct
func DecodePacketAttestation(data []byte) (height uint64, packets []PacketCompact, err error) {
	values, err := packetAttestationArgs.Unpack(data)
	if err != nil {
		return 0, nil, fmt.Errorf("unpack packet attestation: %w", err)
	}

	var decoded struct {
		PacketAttestation packetAttestation
	}
	if err := packetAttestationArgs.Copy(&decoded, values); err != nil {
		return 0, nil, fmt.Errorf("copy packet attestation: %w", err)
	}

	return decoded.PacketAttestation.Height, decoded.PacketAttestation.Packets, nil
}

// EncodePacket encodes a channeltypesv2.Packet as the Solidity ABI
// IICS26RouterMsgs.Packet tuple
func EncodePacket(p channeltypesv2.Packet) ([]byte, error) {
	payloads := make([]payload, len(p.Payloads))
	for i, item := range p.Payloads {
		payloads[i] = payload{
			SourcePort: item.SourcePort,
			DestPort:   item.DestinationPort,
			Version:    item.Version,
			Encoding:   item.Encoding,
			Value:      item.Value,
		}
	}

	data, err := packetArgs.Pack(packet{
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

// DecodePacket decodes a Solidity ABI-encoded IBC v2 packet.
func DecodePacket(data []byte) (channeltypesv2.Packet, error) {
	values, err := packetArgs.Unpack(data)
	if err != nil {
		return channeltypesv2.Packet{}, fmt.Errorf("unpack packet: %w", err)
	}

	var decoded struct {
		Packet packet
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

// EncodeAttestationProof encodes the Solidity AttestationProof struct
func EncodeAttestationProof(p AttestationProof) ([]byte, error) {
	data, err := attestationProofArgs.Pack(p)
	if err != nil {
		return nil, fmt.Errorf("encode attestation proof: %w", err)
	}

	return data, nil
}

func mustNewTuple(components []abi.ArgumentMarshaling) abi.Type {
	typ, err := abi.NewType("tuple", "", components)
	if err != nil {
		panic(err)
	}

	return typ
}
