// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// A concurrent update can move the client past the requested height; the
// packets are then proven at the height the prover returns.
func TestRelayPacketsProvesAtReturnedHeight(t *testing.T) {
	events := []v2.PacketEvent{{Height: 100, Packet: channeltypesv2.Packet{Sequence: 1}}}

	chainClient := mocks.NewMockClient(t)
	chainClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
	chainClient.EXPECT().ChainID().Return("dst").Once()

	mockProver := mocks.NewMockProver(t)
	mockProver.EXPECT().ClientUpdatePayloads(mock.Anything, uint64(100)).Return(nil, uint64(105), nil).Once()
	mockProver.EXPECT().PacketProofs(mock.Anything, uint64(105), v2.ProofKindPacketCommitment, mock.Anything).
		Return([][]byte{{0x02}}, nil).Once()

	txBuilder := mocks.NewMockTxBuilder(t)
	txBuilder.EXPECT().BuildRelayTxs(v2.ClientUpdate{ClientID: "client-0"}, mock.Anything).
		RunAndReturn(func(_ v2.ClientUpdate, items []v2.PacketRelayItem) ([]v2.RelayTx, error) {
			require.Len(t, items, 1)
			require.Equal(t, uint64(105), items[0].ProofHeight)
			return []v2.RelayTx{{To: common.HexToAddress("0x01").Bytes()}}, nil
		}).Once()

	txSubmitter := mocks.NewMockTxSubmitter(t)
	txSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).
		Return(&v2.Submission{TxHash: "0xrecv", SubmittedAt: time.Now()}, nil).Once()

	_, err := relayPackets(
		context.Background(), slog.Default(), chainClient, mockProver, txBuilder, txSubmitter,
		"client-0", v2.RelayKindRecv, 100, events,
	)
	require.NoError(t, err)
}

func TestRelayPacketsRejectsLowerProofHeight(t *testing.T) {
	events := []v2.PacketEvent{{Height: 100, Packet: channeltypesv2.Packet{Sequence: 1}}}

	mockProver := mocks.NewMockProver(t)
	mockProver.EXPECT().ClientUpdatePayloads(mock.Anything, uint64(100)).Return(nil, uint64(99), nil).Once()

	_, err := relayPackets(
		context.Background(), slog.Default(), mocks.NewMockClient(t), mockProver,
		mocks.NewMockTxBuilder(t), mocks.NewMockTxSubmitter(t),
		"client-0", v2.RelayKindRecv, 100, events,
	)
	require.ErrorContains(t, err, "prover returned proof height 99 below requested 100")
}
