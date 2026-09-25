// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"errors"
	"math/big"
	"testing"

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

func TestBesuQBFTHead(t *testing.T) {
	ctx := context.Background()
	update := besutest.MustFixture(t).AdjacentUpdate
	var sealed types.Header
	require.NoError(t, rlp.DecodeBytes(update.HeaderRLP, &sealed))

	t.Run("sealed head", func(t *testing.T) {
		eth := mocks.NewMockETHClient(t)
		eth.EXPECT().HeaderByNumber(ctx, (*big.Int)(nil)).Return(&sealed, nil).Once()

		height, state, err := (&Driver{backend: eth}).BesuQBFTHead(ctx)
		require.NoError(t, err)
		require.Equal(t, update.Height, height)
		require.Equal(t, update.ExpectedConsensusState(), state)
	})

	t.Run("genesis head", func(t *testing.T) {
		genesis := sealed
		genesis.Number = big.NewInt(0)
		eth := mocks.NewMockETHClient(t)
		eth.EXPECT().HeaderByNumber(ctx, (*big.Int)(nil)).Return(&genesis, nil).Once()

		_, _, err := (&Driver{backend: eth}).BesuQBFTHead(ctx)
		require.ErrorContains(t, err, "no block past genesis")
	})

	t.Run("not a besu header", func(t *testing.T) {
		eth := mocks.NewMockETHClient(t)
		eth.EXPECT().HeaderByNumber(ctx, (*big.Int)(nil)).
			Return(&types.Header{Number: big.NewInt(7), Difficulty: big.NewInt(1)}, nil).Once()

		_, _, err := (&Driver{backend: eth}).BesuQBFTHead(ctx)
		require.ErrorContains(t, err, "parse header latest: extra data list")
	})

	t.Run("rpc error", func(t *testing.T) {
		eth := mocks.NewMockETHClient(t)
		eth.EXPECT().HeaderByNumber(ctx, (*big.Int)(nil)).Return(nil, errors.New("boom")).Once()

		_, _, err := (&Driver{backend: eth}).BesuQBFTHead(ctx)
		require.ErrorContains(t, err, "getting header latest")
	})
}
