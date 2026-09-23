// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/deploy"
)

func TestAttestationArgs(t *testing.T) {
	attestors, err := attestationArgs(deploy.AttestationParams{
		Attestors: []string{
			"0x00000000000000000000000000000000000000aa",
			"0x00000000000000000000000000000000000000bb",
		},
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

	_, err = attestationArgs(
		deploy.AttestationParams{Attestors: []string{"nothex"}, Threshold: 1, InitialHeight: 1, InitialTimestamp: 1},
	)
	require.ErrorContains(t, err, "invalid attestor address")

	_, err = attestationArgs(
		deploy.AttestationParams{
			Attestors:        []string{"0x00000000000000000000000000000000000000aa"},
			Threshold:        2,
			InitialHeight:    1,
			InitialTimestamp: 1,
		},
	)
	require.ErrorContains(t, err, "threshold")
}

func TestReadOnlyDriverGuards(t *testing.T) {
	d := &Driver{chainID: big.NewInt(1)}
	_, err := d.ProvisionCore(context.Background(), deploy.CoreParams{})
	require.ErrorContains(t, err, "no deployer signer configured")
	_, err = d.ProvisionClient(
		context.Background(),
		common.Address{}.Hex(),
		deploy.ClientSpec{
			Type: deploy.ClientTypeAttestation,
			Params: deploy.AttestationParams{
				Attestors:        []string{"0x00000000000000000000000000000000000000aa"},
				Threshold:        1,
				InitialHeight:    1,
				InitialTimestamp: 1,
			},
		},
	)
	require.ErrorContains(t, err, "no deployer signer configured")
}

func TestBesuQBFTArgs(t *testing.T) {
	valid := deploy.BesuQBFTParams{
		IBCRouter:        "0x00000000000000000000000000000000000000cc",
		InitialHeight:    1,
		InitialTimestamp: 1,
		InitialStateRoot: "0x69c8d1758a0375ec0d4ee22f16e3119c84ecb3aaaaaaaaaaaaaaaaaaaaaaaaaa",
		InitialValidators: []string{
			"0x00000000000000000000000000000000000000aa",
			"0x00000000000000000000000000000000000000bb",
		},
	}

	args, err := besuQBFTArgs(valid)
	require.NoError(t, err)
	require.Equal(t, common.HexToAddress(valid.IBCRouter), args.router)
	require.Equal(t, common.HexToHash(valid.InitialStateRoot), common.Hash(args.stateRoot))
	require.Len(t, args.validators, 2)

	for name, tc := range map[string]struct {
		mutate func(*deploy.BesuQBFTParams)
		want   string
	}{
		"router":        {func(p *deploy.BesuQBFTParams) { p.IBCRouter = "nothex" }, "router"},
		"zero router":   {func(p *deploy.BesuQBFTParams) { p.IBCRouter = common.Address{}.Hex() }, "router"},
		"short root":    {func(p *deploy.BesuQBFTParams) { p.InitialStateRoot = "0x1234" }, "32 hex bytes"},
		"bad validator": {func(p *deploy.BesuQBFTParams) { p.InitialValidators = []string{"zz"} }, "validator address"},
	} {
		t.Run(name, func(t *testing.T) {
			p := valid
			p.InitialValidators = append([]string(nil), valid.InitialValidators...)
			tc.mutate(&p)
			_, err := besuQBFTArgs(p)
			require.ErrorContains(t, err, tc.want)
		})
	}
}
