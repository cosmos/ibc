// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu/besutest"
	"github.com/cosmos/ibc/cli/internal/deploy"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
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

// Zero values the constructor accepts are refused before any transaction.
func TestProvisionBesuQBFTRejectsZeroValues(t *testing.T) {
	valid := deploy.BesuQBFTParams{
		IBCRouter:     common.HexToAddress("0x00000000000000000000000000000000000000cc"),
		InitialHeight: 1,
		InitialConsensusState: besumsgs.IBesuLightClientMsgsConsensusState{
			Timestamp:  1,
			StateRoot:  common.HexToHash("0x69c8d1758a0375ec0d4ee22f16e3119c84ecb3aaaaaaaaaaaaaaaaaaaaaaaaaa"),
			Validators: []common.Address{common.HexToAddress("0x00000000000000000000000000000000000000aa")},
		},
	}

	for name, tc := range map[string]struct {
		mutate func(*deploy.BesuQBFTParams)
		want   string
	}{
		"valid":       {func(*deploy.BesuQBFTParams) {}, "no deployer signer configured"},
		"zero router": {func(p *deploy.BesuQBFTParams) { p.IBCRouter = common.Address{} }, "router must not be zero"},
		"zero root": {
			func(p *deploy.BesuQBFTParams) { p.InitialConsensusState.StateRoot = [32]byte{} },
			"initial state root must not be zero",
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := valid
			tc.mutate(&p)
			_, err := (&Driver{chainID: big.NewInt(1)}).provisionBesuQBFT(
				t.Context(),
				common.Address{}.Hex(),
				deploy.ClientSpec{Type: deploy.ClientTypeBesuQBFT, Params: p},
			)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestBesuQBFTConsensusState(t *testing.T) {
	ctx := context.Background()
	update := besutest.MustFixture(t).AdjacentUpdate
	var sealed types.Header
	require.NoError(t, rlp.DecodeBytes(update.HeaderRLP, &sealed))

	t.Run("sealed header", func(t *testing.T) {
		eth := mocks.NewMockETHClient(t)
		eth.EXPECT().HeaderByNumber(ctx, new(big.Int).SetUint64(update.Height)).Return(&sealed, nil).Once()

		state, err := (&Driver{backend: eth}).BesuQBFTConsensusState(ctx, update.Height)
		require.NoError(t, err)
		require.Equal(t, update.ExpectedConsensusState(), state)
	})

	t.Run("not a besu header", func(t *testing.T) {
		eth := mocks.NewMockETHClient(t)
		eth.EXPECT().HeaderByNumber(ctx, big.NewInt(7)).
			Return(&types.Header{Number: big.NewInt(7), Difficulty: big.NewInt(1)}, nil).Once()

		_, err := (&Driver{backend: eth}).BesuQBFTConsensusState(ctx, 7)
		require.ErrorContains(t, err, "not a Besu QBFT header")
	})

	t.Run("rpc error", func(t *testing.T) {
		eth := mocks.NewMockETHClient(t)
		eth.EXPECT().HeaderByNumber(ctx, big.NewInt(7)).Return(nil, errors.New("boom")).Once()

		_, err := (&Driver{backend: eth}).BesuQBFTConsensusState(ctx, 7)
		require.ErrorContains(t, err, "fetch header 7")
	})
}
