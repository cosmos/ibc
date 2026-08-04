package e2e_test

import (
	"testing"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/attestation"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
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
