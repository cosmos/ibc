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

// Every client update the prover returns reaches the tx builder in order, so
// a light client that needs intermediate headers gets them in the same tx as
// the packets they unlock.
func TestRelayPacketsForwardsOrderedClientUpdates(t *testing.T) {
	ctx := context.Background()
	updates := [][]byte{{0xa1}, {0xa2}, {0xa3}}
	events := []v2.PacketEvent{{
		Height: 100, Kind: v2.KindSendPacket,
		Packet: channeltypesv2.Packet{Sequence: 7, SourceClient: "src", DestinationClient: "dst"},
	}}

	chainClient := mocks.NewMockClient(t)
	chainClient.EXPECT().ChainID().Return("destination").Once()
	chainClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()

	mockProver := mocks.NewMockProver(t)
	mockProver.EXPECT().StateProof(mock.Anything, uint64(100)).Return(updates, nil).Once()
	mockProver.EXPECT().PacketProofs(mock.Anything, uint64(100), v2.ProofKindPacketCommitment, mock.Anything).
		Return([][]byte{{0x02}}, nil).Once()

	txBuilder := mocks.NewMockTxBuilder(t)
	txBuilder.EXPECT().BuildRelayTxs(
		v2.ClientUpdate{ClientID: "dst", StateProofs: updates},
		mock.Anything,
	).Return([]v2.RelayTx{{To: common.HexToAddress("0xrouter").Bytes(), Data: []byte{0x01}}}, nil).Once()

	txSubmitter := mocks.NewMockTxSubmitter(t)
	txSubmitter.EXPECT().Submit(mock.Anything, mock.Anything).
		Return(&v2.Submission{TxHash: "0xrecv", SubmittedAt: time.Now().UTC()}, nil).Once()

	submission, err := relayPackets(
		ctx, slog.Default(), chainClient, mockProver, txBuilder, txSubmitter, "dst", v2.RelayKindRecv, 100, events,
	)
	require.NoError(t, err)
	require.Equal(t, "0xrecv", submission.TxHash)
}
