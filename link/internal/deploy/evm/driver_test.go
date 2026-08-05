package evm

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/deploy"
)

func TestChecksumAddress(t *testing.T) {
	lower := "0x00000000000000000000000000000000000000aa"
	want := common.HexToAddress(lower).Hex()
	require.Equal(t, want, checksumAddress(lower))

	// already-checksummed input is unchanged
	require.Equal(t, want, checksumAddress(want))
}

func TestAttestationEnv(t *testing.T) {
	env, err := attestationEnv(deploy.AttestationParams{
		Attestors:        []string{"0x00000000000000000000000000000000000000aa", "0x00000000000000000000000000000000000000bb"},
		Threshold:        2,
		InitialHeight:    42,
		InitialTimestamp: 1700000000,
	})
	require.NoError(t, err)
	require.Equal(t, "0x00000000000000000000000000000000000000aa,0x00000000000000000000000000000000000000bb", env["IBC_ATTESTORS"])
	require.Equal(t, "2", env["IBC_THRESHOLD"])
	require.Equal(t, "42", env["IBC_HEIGHT"])
	require.Equal(t, "1700000000", env["IBC_TIMESTAMP"])

	_, err = attestationEnv(deploy.AttestationParams{Threshold: 1, InitialHeight: 1, InitialTimestamp: 1})
	require.ErrorContains(t, err, "attestors")

	_, err = attestationEnv(deploy.AttestationParams{Attestors: []string{"nothex"}, Threshold: 1, InitialHeight: 1, InitialTimestamp: 1})
	require.ErrorContains(t, err, "invalid attestor address")

	_, err = attestationEnv(deploy.AttestationParams{Attestors: []string{"0x00000000000000000000000000000000000000aa"}, Threshold: 2, InitialHeight: 1, InitialTimestamp: 1})
	require.ErrorContains(t, err, "threshold")
}

func TestReadOnlyDriverGuards(t *testing.T) {
	d := &Driver{chainID: big.NewInt(1)}
	_, err := d.ProvisionCore(context.Background(), deploy.CoreParams{})
	require.ErrorContains(t, err, "no deployer signer configured")
	_, err = d.ProvisionClient(context.Background(), deploy.ClientSpec{Type: deploy.ClientTypeAttestation, Params: deploy.AttestationParams{Attestors: []string{"0x00000000000000000000000000000000000000aa"}, Threshold: 1, InitialHeight: 1, InitialTimestamp: 1}})
	require.ErrorContains(t, err, "no deployer signer configured")
	require.Empty(t, d.DeployerAddress())
}
