// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

// PacketCompact mirrors the Solidity PacketCompact struct.
type PacketCompact struct {
	Path       [32]byte `abi:"path"`
	Commitment [32]byte `abi:"commitment"`
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
