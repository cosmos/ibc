package evm

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

type stateAttestation struct {
	Height    uint64 `abi:"height"`
	Timestamp uint64 `abi:"timestamp"`
}

var stateAttestationArgs = abi.Arguments{{Type: mustNewTuple(
	[]abi.ArgumentMarshaling{
		{Name: "height", Type: "uint64"},
		{Name: "timestamp", Type: "uint64"},
	},
)}}

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

func mustNewTuple(components []abi.ArgumentMarshaling) abi.Type {
	typ, err := abi.NewType("tuple", "", components)
	if err != nil {
		panic(err)
	}

	return typ
}
