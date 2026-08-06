package evm

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/deploy"
)

func TestAttestationArgs(t *testing.T) {
	attestors, err := attestationArgs(deploy.AttestationParams{
		Attestors:        []string{"0x00000000000000000000000000000000000000aa", "0x00000000000000000000000000000000000000bb"},
		Threshold:        2,
		InitialHeight:    42,
		InitialTimestamp: 1700000000,
	})
	require.NoError(t, err)
	require.Equal(t, []common.Address{
		common.HexToAddress("0x00000000000000000000000000000000000000aa"),
		common.HexToAddress("0x00000000000000000000000000000000000000bb"),
	}, attestors)

	_, err = attestationArgs(deploy.AttestationParams{Threshold: 1, InitialHeight: 1, InitialTimestamp: 1})
	require.ErrorContains(t, err, "attestors")

	_, err = attestationArgs(deploy.AttestationParams{Attestors: []string{"nothex"}, Threshold: 1, InitialHeight: 1, InitialTimestamp: 1})
	require.ErrorContains(t, err, "invalid attestor address")

	_, err = attestationArgs(deploy.AttestationParams{Attestors: []string{"0x00000000000000000000000000000000000000aa"}, Threshold: 2, InitialHeight: 1, InitialTimestamp: 1})
	require.ErrorContains(t, err, "threshold")
}

func TestAccessManagerArtifact(t *testing.T) {
	amABI, bin, err := accessManagerArtifact()
	require.NoError(t, err)
	require.NotEmpty(t, bin)
	require.Contains(t, amABI.Methods, "setTargetFunctionRole")
}

func TestReadOnlyDriverGuards(t *testing.T) {
	d := &Driver{chainID: big.NewInt(1)}
	_, err := d.ProvisionCore(context.Background(), deploy.CoreParams{})
	require.ErrorContains(t, err, "no deployer signer configured")
	_, err = d.ProvisionClient(context.Background(), common.Address{}.Hex(), deploy.ClientSpec{Type: deploy.ClientTypeAttestation, Params: deploy.AttestationParams{Attestors: []string{"0x00000000000000000000000000000000000000aa"}, Threshold: 1, InitialHeight: 1, InitialTimestamp: 1}})
	require.ErrorContains(t, err, "no deployer signer configured")
}
