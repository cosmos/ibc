// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"fmt"
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

type relayEnv struct {
	chain     *mocks.MockClient
	prover    *mocks.MockProver
	builder   *mocks.MockTxBuilder
	submitter *mocks.MockTxSubmitter
	events    []v2.PacketEvent
}

func newRelayEnv(t *testing.T) *relayEnv {
	t.Helper()
	e := &relayEnv{
		chain: mocks.NewMockClient(t), prover: mocks.NewMockProver(t),
		builder: mocks.NewMockTxBuilder(t), submitter: mocks.NewMockTxSubmitter(t),
		events: []v2.PacketEvent{{
			Height: 100, Kind: v2.KindSendPacket,
			Packet: channeltypesv2.Packet{Sequence: 7, SourceClient: "src", DestinationClient: "dst"},
		}},
	}
	e.chain.EXPECT().ChainID().Return("destination")
	return e
}

func (e *relayEnv) run(ctx context.Context) (*v2.Submission, error) {
	return relayPackets(
		ctx, slog.Default(), e.chain, e.prover, e.builder, e.submitter, "dst", v2.RelayKindRecv, 100, e.events,
	)
}

func (e *relayEnv) ready(update []byte) {
	e.prover.EXPECT().Prepare(mock.Anything, uint64(100), v2.ProofKindPacketCommitment, mock.Anything).
		Return(&v2.Preparation{Ready: &v2.BatchProofs{Update: update, PacketProofs: [][]byte{{2}}}}, nil).Once()
}

func (e *relayEnv) packetItems() []v2.PacketRelayItem {
	return []v2.PacketRelayItem{{Kind: v2.RelayKindRecv, Packet: e.events[0].Packet, Proof: []byte{2}, ProofHeight: 100}}
}

func TestRelayPacketsConfirmsOrderedCheckpoints(t *testing.T) {
	e := newRelayEnv(t)
	for _, update := range []byte{0xa1, 0xa2} {
		e.prover.EXPECT().Prepare(mock.Anything, uint64(100), v2.ProofKindPacketCommitment, mock.Anything).
			Return(&v2.Preparation{Advance: []byte{update}}, nil).Once()
	}
	e.ready([]byte{0xa3})
	var batches []int
	e.builder.EXPECT().BuildRelayTx(mock.Anything, mock.Anything).RunAndReturn(
		func(update v2.ClientUpdate, items []v2.PacketRelayItem) (v2.RelayTx, error) {
			batches = append(batches, len(items))
			return v2.RelayTx{To: common.HexToAddress("0x1234").Bytes(), Data: update.Proof}, nil
		}).Times(3)
	e.chain.EXPECT().WaitForChain(mock.Anything).Return(nil).Times(3)
	var confirmed []string
	for _, code := range []byte{0xa1, 0xa2} {
		hash := fmt.Sprintf("0x%x", code)
		e.submitter.EXPECT().Submit(mock.Anything, mock.MatchedBy(func(intent v2.TxIntent) bool {
			return len(intent.Data) == 1 && intent.Data[0] == code
		})).RunAndReturn(func(context.Context, v2.TxIntent) (*v2.Submission, error) {
			// No next checkpoint is submitted before the preceding one is confirmed.
			require.Len(t, confirmed, len(batches)-1)
			return &v2.Submission{TxHash: hash, SubmittedAt: time.Now().UTC()}, nil
		}).Once()
		e.submitter.EXPECT().ShouldRetry(mock.Anything, hash, mock.Anything).
			Run(func(context.Context, string, time.Time) { confirmed = append(confirmed, hash) }).
			Return(false, nil).Once()
	}
	e.submitter.EXPECT().Submit(mock.Anything, v2.TxIntent{To: common.HexToAddress("0x1234").Hex(), Data: []byte{0xa3}}).
		Return(&v2.Submission{TxHash: "0xpacket"}, nil).Once()
	result, err := e.run(t.Context())
	require.NoError(t, err)
	require.Equal(t, "0xpacket", result.TxHash)
	require.Equal(t, []int{0, 0, 1}, batches)
}

func TestRelayPacketsWaitsForPendingCheckpoint(t *testing.T) {
	e := newRelayEnv(t)
	e.prover.EXPECT().Prepare(mock.Anything, uint64(100), v2.ProofKindPacketCommitment, mock.Anything).
		Return(&v2.Preparation{Advance: []byte{1}}, nil).Once()
	e.builder.EXPECT().BuildRelayTx(mock.Anything, mock.Anything).
		Return(v2.RelayTx{To: common.HexToAddress("0x1234").Bytes(), Data: []byte{1}}, nil).Once()
	e.chain.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
	e.submitter.EXPECT().Submit(mock.Anything, mock.Anything).
		Return(&v2.Submission{TxHash: "checkpoint", SubmittedAt: time.Now().UTC()}, nil).Once()
	ctx, cancel := context.WithCancel(t.Context())
	e.submitter.EXPECT().ShouldRetry(mock.Anything, "checkpoint", mock.Anything).
		Run(func(context.Context, string, time.Time) { cancel() }).Return(false, v2.ErrTxNotFound).Once()
	_, err := e.run(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestFailedCheckpointDoesNotSubmitPackets(t *testing.T) {
	e := newRelayEnv(t)
	e.prover.EXPECT().Prepare(mock.Anything, uint64(100), v2.ProofKindPacketCommitment, mock.Anything).
		Return(&v2.Preparation{Advance: []byte{1}}, nil).Once()
	e.builder.EXPECT().BuildRelayTx(mock.Anything, mock.Anything).
		Return(v2.RelayTx{To: common.HexToAddress("0x1234").Bytes(), Data: []byte{1}}, nil).Once()
	e.chain.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
	e.submitter.EXPECT().Submit(mock.Anything, mock.Anything).
		Return(&v2.Submission{TxHash: "failed", SubmittedAt: time.Now().UTC()}, nil).Once()
	e.submitter.EXPECT().ShouldRetry(mock.Anything, "failed", mock.Anything).Return(true, nil).Once()
	_, err := e.run(t.Context())
	require.ErrorContains(t, err, "failed or expired")
}

func TestRelayPacketsSubmitsUpdateWithPackets(t *testing.T) {
	e := newRelayEnv(t)
	e.ready([]byte{1})
	e.builder.EXPECT().BuildRelayTx(v2.ClientUpdate{ClientID: "dst", Proof: []byte{1}}, e.packetItems()).
		Return(v2.RelayTx{To: common.HexToAddress("0x1234").Bytes(), Data: []byte{1, 2}}, nil).Once()
	e.chain.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
	e.submitter.EXPECT().Submit(mock.Anything, mock.Anything).Return(&v2.Submission{TxHash: "atomic"}, nil).Once()
	result, err := e.run(t.Context())
	require.NoError(t, err)
	require.Equal(t, "atomic", result.TxHash)
}

func TestRelayPacketsRejectsInvalidPreparation(t *testing.T) {
	e := newRelayEnv(t)
	e.prover.EXPECT().Prepare(mock.Anything, uint64(100), v2.ProofKindPacketCommitment, mock.Anything).
		Return(&v2.Preparation{Ready: &v2.BatchProofs{}}, nil).Once()
	_, err := e.run(t.Context())
	require.ErrorContains(t, err, "0 proofs for 1 packets")
}
