// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/cli/internal/service/signer"
	"github.com/cosmos/ibc/cli/internal/store"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
	"github.com/cosmos/ibc/cli/internal/txsubmitter"
	"github.com/cosmos/ibc/cli/internal/txsubmitter/evm"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

type relayEnv struct {
	db        *store.SqliteDB
	chain     *mocks.MockClient
	prover    *mocks.MockProver
	builder   *mocks.MockTxBuilder
	submitter *mocks.MockTxSubmitter
	events    []v2.PacketEvent
}

func newRelayEnv(t *testing.T) *relayEnv {
	t.Helper()
	db, err := store.NewSqliteInMemory()
	require.NoError(t, err)
	_, err = db.MigrateUp()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	e := &relayEnv{
		db: db, chain: mocks.NewMockClient(t), prover: mocks.NewMockProver(t),
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
	return e.runWithSubmitter(ctx, e.submitter)
}

func (e *relayEnv) runWithSubmitter(ctx context.Context, submitter txsubmitter.TxSubmitter) (*v2.Submission, error) {
	return relayPackets(
		ctx,
		slog.Default(),
		e.chain,
		e.prover,
		e.builder,
		submitter,
		e.db,
		"dst",
		v2.RelayKindRecv,
		100,
		e.events,
	)
}

type checkpointRPCError struct {
	code    int
	message string
}

func (e checkpointRPCError) Error() string  { return e.message }
func (e checkpointRPCError) ErrorCode() int { return e.code }

func TestCheckpointBroadcastFailureRecovery(t *testing.T) {
	for _, tc := range []struct {
		name     string
		sendErr  error
		rejected bool
	}{
		{"besu rejected", checkpointRPCError{-32004, "Upfront cost exceeds account balance"}, true},
		{"geth rejected", checkpointRPCError{-32000, "exceeds block gas limit"}, true},
		{"transport uncertain", io.ErrUnexpectedEOF, false},
		{"canceled", context.Canceled, false},
		{"nonce too low", checkpointRPCError{-32001, "Nonce too low"}, false},
		{"already known", checkpointRPCError{-32000, "already known"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newRelayEnv(t)
			eth := mocks.NewMockTxSubmitterETHClient(t)
			chainSigner, err := signer.GenerateLocalSecp256k1Signer()
			require.NoError(t, err)
			submitter, err := evm.New("1", eth, chainSigner, evm.ChainOptions{TxSubmissionDelay: time.Millisecond})
			require.NoError(t, err)
			e.prover.EXPECT().Prepare(mock.Anything, uint64(100), mock.Anything, mock.Anything).
				Return(&v2.Preparation{Advance: []byte{1}}, nil).Once()
			e.builder.EXPECT().BuildRelayTx(mock.Anything, mock.Anything).
				RunAndReturn(func(_ v2.ClientUpdate, items []v2.PacketRelayItem) (v2.RelayTx, error) {
					return v2.RelayTx{
						To:   common.HexToAddress("0x1234").Bytes(),
						Data: []byte{byte(len(items) + 1)},
					}, nil
				}).Twice()
			e.chain.EXPECT().WaitForChain(mock.Anything).Return(nil).Twice()
			eth.EXPECT().HeaderByNumber(mock.Anything, (*big.Int)(nil)).
				Return(&types.Header{BaseFee: big.NewInt(100), GasLimit: 30000000}, nil).Twice()
			eth.EXPECT().SuggestGasTipCap(mock.Anything).Return(big.NewInt(10), nil).Twice()
			eth.EXPECT().PendingCodeAt(mock.Anything, mock.Anything).Return([]byte{1}, nil).Twice()
			eth.EXPECT().EstimateGas(mock.Anything, mock.Anything).Return(uint64(21000), nil).Twice()
			eth.EXPECT().PendingNonceAt(mock.Anything, mock.Anything).Return(uint64(7), nil).Twice()
			sendCtx, cancelSend := context.WithCancel(t.Context())
			defer cancelSend()
			var checkpoint common.Hash
			eth.EXPECT().SendTransaction(mock.Anything, mock.Anything).
				Run(func(ctx context.Context, tx *types.Transaction) {
					checkpoint = tx.Hash()
					pending, loadErr := e.db.GetClientUpdate(ctx, "destination", "dst")
					require.NoError(t, loadErr)
					require.NotNil(t, pending, "the signed identity must precede the send attempt")
					require.Equal(t, checkpoint.Hex(), pending.Hash)
					if tc.name == "canceled" {
						cancelSend()
					}
				}).Return(tc.sendErr).Once()
			result, err := e.runWithSubmitter(sendCtx, submitter)
			require.Nil(t, result)
			require.ErrorIs(t, err, tc.sendErr)
			pending, loadErr := e.db.GetClientUpdate(t.Context(), "destination", "dst")
			require.NoError(t, loadErr)
			if tc.rejected {
				require.ErrorIs(t, err, v2.ErrTxRejected)
				require.Nil(t, pending)
				// No receipt lookup is expected: a rejected checkpoint cannot stall this relay.
			} else {
				require.NotErrorIs(t, err, v2.ErrTxRejected)
				require.NotNil(t, pending)
				require.Equal(t, checkpoint.Hex(), pending.Hash)
				recovered := eth.EXPECT().TransactionReceipt(mock.Anything, checkpoint).
					Return(&types.Receipt{TxHash: checkpoint, Status: types.ReceiptStatusSuccessful}, nil).Once()
				e.prover.EXPECT().Prepare(mock.Anything, uint64(100), mock.Anything, mock.Anything).
					Return(&v2.Preparation{Ready: &v2.BatchProofs{PacketProofs: [][]byte{{2}}}}, nil).
					NotBefore(recovered).Once()
			}
			if tc.rejected {
				e.ready()
			}
			eth.EXPECT().SendTransaction(mock.Anything, mock.Anything).Return(nil).Once()
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			result, err = e.runWithSubmitter(ctx, submitter)
			require.NoError(t, err)
			require.NotNil(t, result)
			pending, err = e.db.GetClientUpdate(t.Context(), "destination", "dst")
			require.NoError(t, err)
			require.Nil(t, pending)
		})
	}
}

func (e *relayEnv) ready() {
	e.prover.EXPECT().Prepare(mock.Anything, uint64(100), v2.ProofKindPacketCommitment, mock.Anything).
		Return(&v2.Preparation{Ready: &v2.BatchProofs{PacketProofs: [][]byte{{2}}}}, nil).Once()
}

func TestRelayPacketsConfirmsOrderedCheckpoints(t *testing.T) {
	e := newRelayEnv(t)
	for _, update := range []byte{0xa1, 0xa2} {
		e.prover.EXPECT().Prepare(mock.Anything, uint64(100), v2.ProofKindPacketCommitment, mock.Anything).
			Return(&v2.Preparation{Advance: []byte{update}}, nil).Once()
	}
	e.ready()
	var batches []int
	e.builder.EXPECT().BuildRelayTx(mock.Anything, mock.Anything).RunAndReturn(
		func(update v2.ClientUpdate, items []v2.PacketRelayItem) (v2.RelayTx, error) {
			batches = append(batches, len(items))
			data := []byte{0xff}
			if len(update.Proof) > 0 {
				data = update.Proof
			}
			return v2.RelayTx{To: common.HexToAddress("0x1234").Bytes(), Data: data}, nil
		}).Times(3)
	e.chain.EXPECT().WaitForChain(mock.Anything).Return(nil).Times(3)
	var confirmed []string
	for _, code := range []byte{0xa1, 0xa2} {
		hash := fmt.Sprintf("0x%x", code)
		e.submitter.EXPECT().Submit(mock.Anything, mock.MatchedBy(func(intent v2.TxIntent) bool {
			return len(intent.Data) == 1 && intent.Data[0] == code
		}), mock.Anything).RunAndReturn(func(ctx context.Context, _ v2.TxIntent, record func(*v2.Submission) error) (*v2.Submission, error) {
			// No next checkpoint is submitted before the preceding one is confirmed.
			require.Len(t, confirmed, len(batches)-1)
			sub := &v2.Submission{TxHash: hash, SubmittedAt: time.Now().UTC()}
			require.NotNil(t, record)
			require.NoError(t, record(sub))
			pending, err := e.db.GetClientUpdate(ctx, "destination", "dst")
			require.NoError(t, err)
			require.Equal(t, hash, pending.Hash)
			return sub, nil
		}).Once()
		e.submitter.EXPECT().
			ShouldRetry(mock.Anything, hash, mock.Anything).
			Run(func(context.Context, string, time.Time) {
				confirmed = append(confirmed, hash)
			}).
			Return(false, nil).
			Once()
	}
	e.submitter.EXPECT().
		Submit(mock.Anything, mock.Anything, mock.MatchedBy(func(record func(*v2.Submission) error) bool { return record == nil })).
		Return(&v2.Submission{TxHash: "0xpacket"}, nil).
		Once()
	result, err := e.run(t.Context())
	require.NoError(t, err)
	require.Equal(t, "0xpacket", result.TxHash)
	require.Equal(t, []int{0, 0, 1}, batches)
	pending, err := e.db.GetClientUpdate(t.Context(), "destination", "dst")
	require.NoError(t, err)
	require.Nil(t, pending)
}

func TestRelayRecoversPendingCheckpointBeforePreparing(t *testing.T) {
	e := newRelayEnv(t)
	pending := store.PacketTx{Hash: "0xprevious-process", Time: time.Now().UTC()}
	require.NoError(t, e.db.SaveClientUpdate(t.Context(), "destination", "dst", pending))
	ctx, cancel := context.WithCancel(t.Context())
	e.submitter.EXPECT().ShouldRetry(mock.Anything, pending.Hash, mock.Anything).
		Run(func(context.Context, string, time.Time) { cancel() }).Return(false, v2.ErrTxNotFound).Once()
	_, err := e.run(ctx)
	require.ErrorIs(t, err, context.Canceled)
	saved, err := e.db.GetClientUpdate(t.Context(), "destination", "dst")
	require.NoError(t, err)
	require.Equal(t, pending.Hash, saved.Hash)

	// A new invocation recovers the saved receipt before doing any proof work.
	e.submitter.EXPECT().ShouldRetry(mock.Anything, pending.Hash, mock.Anything).Return(false, nil).Once()
	e.ready()
	e.builder.EXPECT().BuildRelayTx(mock.Anything, mock.Anything).
		Return(v2.RelayTx{To: common.HexToAddress("0x1234").Bytes(), Data: []byte{1}}, nil).Once()
	e.chain.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
	e.submitter.EXPECT().
		Submit(mock.Anything, mock.Anything, mock.MatchedBy(func(record func(*v2.Submission) error) bool { return record == nil })).
		Return(&v2.Submission{TxHash: "packet"}, nil).
		Once()
	_, err = e.run(t.Context())
	require.NoError(t, err)
}

func TestFailedCheckpointDoesNotSubmitPackets(t *testing.T) {
	e := newRelayEnv(t)
	require.NoError(
		t,
		e.db.SaveClientUpdate(t.Context(), "destination", "dst", store.PacketTx{Hash: "failed", Time: time.Now()}),
	)
	e.submitter.EXPECT().ShouldRetry(mock.Anything, "failed", mock.Anything).Return(true, nil).Once()
	_, err := e.run(t.Context())
	require.ErrorContains(t, err, "failed or expired")
	saved, err := e.db.GetClientUpdate(t.Context(), "destination", "dst")
	require.NoError(t, err)
	require.Nil(t, saved)
}

func TestFinalCheckpointPrecedesPacketsWithoutEstimatingCombinedTx(t *testing.T) {
	for _, outcome := range []string{"confirmed", "failed", "Internal error", "Execution reverted"} {
		t.Run(outcome, func(t *testing.T) {
			e := newRelayEnv(t)
			e.prover.EXPECT().Prepare(mock.Anything, uint64(100), mock.Anything, mock.Anything).
				Return(&v2.Preparation{Ready: &v2.BatchProofs{
					Update: []byte{1}, Checkpoint: true, PacketProofs: [][]byte{{2}},
				}}, nil).Once()
			// No combined transaction is even built or estimated.
			build := e.builder.EXPECT().BuildRelayTx(v2.ClientUpdate{ClientID: "dst", Proof: []byte{1}},
				[]v2.PacketRelayItem(nil)).
				Return(v2.RelayTx{To: common.HexToAddress("0x1234").Bytes(), Data: []byte{1}}, nil).Once()
			e.chain.EXPECT().WaitForChain(mock.Anything).Return(nil)
			e.submitter.EXPECT().
				Submit(mock.Anything, mock.Anything, mock.MatchedBy(func(record func(*v2.Submission) error) bool { return record != nil })).
				RunAndReturn(func(_ context.Context, _ v2.TxIntent, record func(*v2.Submission) error) (*v2.Submission, error) {
					sub := &v2.Submission{TxHash: "checkpoint", SubmittedAt: time.Now().UTC()}
					require.NoError(t, record(sub))
					return sub, nil
				}).
				Once()
			confirm := e.submitter.EXPECT().ShouldRetry(mock.Anything, "checkpoint", mock.Anything).
				Return(outcome == "failed", nil).Once()
			if outcome != "failed" {
				// Reuse the prepared snapshot; don't fetch proofs again after confirmation.
				packetBuild := e.builder.EXPECT().BuildRelayTx(v2.ClientUpdate{ClientID: "dst"},
					[]v2.PacketRelayItem{{
						Kind: v2.RelayKindRecv, Packet: e.events[0].Packet, Proof: []byte{2}, ProofHeight: 100,
					}}).
					Return(v2.RelayTx{To: common.HexToAddress("0x1234").Bytes(), Data: []byte{2}}, nil).Once()
				packetBuild.NotBefore(build, confirm)
				var packetErr error
				var sub *v2.Submission
				if outcome == "confirmed" {
					sub = &v2.Submission{TxHash: "packet"}
				} else {
					packetErr = fmt.Errorf("%s", outcome)
				}
				e.submitter.EXPECT().
					Submit(mock.Anything, mock.Anything, mock.MatchedBy(func(record func(*v2.Submission) error) bool { return record == nil })).
					Return(sub, packetErr).Once()
			}
			result, err := e.run(t.Context())
			if outcome == "confirmed" {
				require.NoError(t, err)
				require.Equal(t, "packet", result.TxHash)
			} else {
				require.ErrorContains(t, err, outcome)
				require.Nil(t, result)
			}
			pending, err := e.db.GetClientUpdate(t.Context(), "destination", "dst")
			require.NoError(t, err)
			require.Nil(t, pending)
		})
	}
}

func TestAtomicUpdateRemainsWithPackets(t *testing.T) {
	e := newRelayEnv(t)
	e.prover.EXPECT().Prepare(mock.Anything, uint64(100), mock.Anything, mock.Anything).
		Return(&v2.Preparation{Ready: &v2.BatchProofs{Update: []byte{1}, PacketProofs: [][]byte{{2}}}}, nil).Once()
	e.builder.EXPECT().BuildRelayTx(v2.ClientUpdate{ClientID: "dst", Proof: []byte{1}},
		[]v2.PacketRelayItem{{Kind: v2.RelayKindRecv, Packet: e.events[0].Packet, Proof: []byte{2}, ProofHeight: 100}}).
		Return(v2.RelayTx{To: common.HexToAddress("0x1234").Bytes(), Data: []byte{1, 2}}, nil).Once()
	e.chain.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
	e.submitter.EXPECT().
		Submit(mock.Anything, mock.Anything, mock.MatchedBy(func(record func(*v2.Submission) error) bool { return record == nil })).
		Return(&v2.Submission{TxHash: "atomic"}, nil).Once()
	result, err := e.run(t.Context())
	require.NoError(t, err)
	require.Equal(t, "atomic", result.TxHash)
}
