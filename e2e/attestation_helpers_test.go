// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"testing"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/attestation"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	attestorevm "github.com/cosmos/ibc/link/attestor/evm"
)

type attestationClientState struct {
	AttestorAddresses []common.Address
	MinRequiredSigs   uint8
	LatestHeight      uint64
	IsFrozen          bool
}

func attestedClientState(
	t testing.TB,
	evm *environment.EVM,
	address environment.EVMAddress,
) attestationClientState {
	t.Helper()
	var encoded []byte
	err := evm.UseContractCaller(func(caller bind.ContractCaller) error {
		client, err := attestation.NewContractCaller(common.HexToAddress(string(address)), caller)
		if err != nil {
			return err
		}
		encoded, err = client.GetClientState(&bind.CallOpts{Context: t.Context()})
		return err
	})
	require.NoError(t, err)
	tuple, err := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
		{Name: "attestorAddresses", Type: "address[]"},
		{Name: "minRequiredSigs", Type: "uint8"},
		{Name: "latestHeight", Type: "uint64"},
		{Name: "isFrozen", Type: "bool"},
	})
	require.NoError(t, err)
	values, err := (abi.Arguments{{Type: tuple}}).Unpack(encoded)
	require.NoError(t, err)
	return *abi.ConvertType(values[0], new(attestationClientState)).(*attestationClientState)
}

func attestedConsensusTimestamp(
	t testing.TB,
	evm *environment.EVM,
	address environment.EVMAddress,
	height uint64,
) uint64 {
	t.Helper()
	var timestamp uint64
	err := evm.UseContractCaller(func(caller bind.ContractCaller) error {
		client, err := attestation.NewContractCaller(common.HexToAddress(string(address)), caller)
		if err != nil {
			return err
		}
		timestamp, err = client.GetConsensusTimestamp(&bind.CallOpts{Context: t.Context()}, height)
		return err
	})
	require.NoError(t, err)
	return timestamp
}

func signedStateAttestationProof(
	t testing.TB,
	privateKeyHex string,
	height, timestamp uint64,
) []byte {
	t.Helper()
	attestationData, err := attestorevm.EncodeStateAttestation(height, timestamp)
	require.NoError(t, err)

	digest := attestorevm.Digest(attestorevm.TagStateAttestation, attestationData)
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	require.NoError(t, err)
	signature, err := crypto.Sign(digest[:], privateKey)
	require.NoError(t, err)
	signature[64] += 27

	proof, err := attestorevm.EncodeAttestationProof(attestationData, [][]byte{signature})
	require.NoError(t, err)
	return proof
}
