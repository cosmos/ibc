// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

func TestValidateRelaySimulation(t *testing.T) {
	update := v2.OperationResult{Type: v2.OperationTypeUpdate, Success: true}
	misbehavior := v2.OperationResult{Type: v2.OperationTypeUpdate, Success: true, Misbehavior: true}
	failedUpdate := v2.OperationResult{Type: v2.OperationTypeUpdate, FailureReason: "boom"}
	packet := v2.OperationResult{Type: v2.OperationTypePacket, Success: true}
	failedPacket := v2.OperationResult{Type: v2.OperationTypePacket, FailureReason: "boom"}

	for _, tc := range []struct {
		name    string
		ops     []v2.OperationResult
		wantErr string
	}{
		{"update and packet", []v2.OperationResult{update, packet}, ""},
		{"update only", []v2.OperationResult{update}, ""},
		{"one packet succeeds", []v2.OperationResult{packet, failedPacket}, ""},
		{"no packet succeeds", []v2.OperationResult{update, failedPacket}, "no packet calls"},
		{"update only fails", []v2.OperationResult{failedUpdate}, "no client update calls"},
		{"misbehavior with packets", []v2.OperationResult{misbehavior, packet}, "would report misbehavior"},
		{"misbehavior after another update", []v2.OperationResult{update, misbehavior}, "would report misbehavior"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRelaySimulation(t.Context(), slog.Default(), v2.RelaySimulation{Operations: tc.ops})
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// byteTxBuilder encodes a tx as its packet count followed by the first byte
// of each update payload.
type byteTxBuilder struct{}

func (byteTxBuilder) BuildRelayTxs(update v2.ClientUpdate, items []v2.PacketRelayItem) ([]v2.RelayTx, error) {
	data := []byte{byte(len(items))}
	for _, payload := range update.Payloads {
		data = append(data, payload[0])
	}
	return []v2.RelayTx{{Data: data}}, nil
}

// recordingTxSubmitter records submissions and inclusion checks in order.
type recordingTxSubmitter struct {
	misbehavior map[byte]bool
	reverted    map[string]bool
	// pollErrs are returned by the first inclusion checks, in order.
	pollErrs []error
	// lookupErr, if set, is returned by every later inclusion check.
	lookupErr error
	submitted int
	log       []string
}

func (s *recordingTxSubmitter) SimulateRelayTx(_ context.Context, tx v2.RelayTx) (v2.RelaySimulation, error) {
	var ops []v2.OperationResult
	for _, id := range tx.Data[1:] {
		ops = append(
			ops,
			v2.OperationResult{Type: v2.OperationTypeUpdate, Success: true, Misbehavior: s.misbehavior[id]},
		)
	}
	for range tx.Data[0] {
		ops = append(ops, v2.OperationResult{Type: v2.OperationTypePacket, Success: true})
	}
	return v2.RelaySimulation{Operations: ops}, nil
}

func (s *recordingTxSubmitter) Submit(_ context.Context, intent v2.TxIntent) (*v2.Submission, error) {
	s.submitted++
	hash := fmt.Sprintf("0x%d", s.submitted)
	s.log = append(s.log, fmt.Sprintf("submit %s updates=%v packets=%d", hash, intent.Data[1:], intent.Data[0]))
	return &v2.Submission{TxHash: hash, SubmittedAt: time.Now()}, nil
}

func (s *recordingTxSubmitter) ShouldRetry(_ context.Context, txHash string, _ time.Time) (bool, error) {
	if len(s.pollErrs) > 0 {
		err := s.pollErrs[0]
		s.pollErrs = s.pollErrs[1:]
		return false, err
	}
	if s.lookupErr != nil {
		return false, s.lookupErr
	}
	s.log = append(s.log, "mined "+txHash)
	return s.reverted[txHash], nil
}

func TestRelayPacketsClientUpdates(t *testing.T) {
	for _, tc := range []struct {
		name      string
		updates   int
		submitter *recordingTxSubmitter
		wantLog   []string
		// wantLines are fragments of the lines logged, in order.
		wantLines []string
		wantErr   string
	}{
		{
			name:      "no update",
			submitter: &recordingTxSubmitter{},
			wantLog:   []string{"submit 0x1 updates=[] packets=1"},
		},
		{
			name:      "one update goes with the packets",
			updates:   1,
			submitter: &recordingTxSubmitter{},
			wantLog:   []string{"submit 0x1 updates=[1] packets=1"},
		},
		{
			name:      "all updates but the last go first, one per transaction",
			updates:   3,
			submitter: &recordingTxSubmitter{pollErrs: []error{v2.ErrTxNotFound}},
			wantLog: []string{
				"submit 0x1 updates=[1] packets=0", "mined 0x1",
				"submit 0x2 updates=[2] packets=0", "mined 0x2",
				"submit 0x3 updates=[3] packets=1",
			},
			wantLines: []string{"update=1 of=3 txHash=0x1", "update=2 of=3 txHash=0x2"},
		},
		{
			name:      "a reverted update stops the relay",
			updates:   3,
			submitter: &recordingTxSubmitter{reverted: map[string]bool{"0x1": true}},
			wantLog:   []string{"submit 0x1 updates=[1] packets=0", "mined 0x1"},
			wantErr:   "client update 1 of 3: client update tx 0x1 reverted or was not mined in time",
		},
		{
			name:      "a failed receipt lookup is polled again",
			updates:   3,
			submitter: &recordingTxSubmitter{pollErrs: []error{errors.New("connection refused")}},
			wantLog: []string{
				"submit 0x1 updates=[1] packets=0", "mined 0x1",
				"submit 0x2 updates=[2] packets=0", "mined 0x2",
				"submit 0x3 updates=[3] packets=1",
			},
			wantLines: []string{
				`msg="Checking client update tx failed, polling again"`, `txHash=0x1 error="connection refused"`,
			},
		},
		{
			name:      "receipt lookups failing until the timeout stop the relay",
			updates:   3,
			submitter: &recordingTxSubmitter{lookupErr: errors.New("connection refused")},
			wantLog:   []string{"submit 0x1 updates=[1] packets=0"},
			wantErr:   "client update 1 of 3: checking client update tx 0x1: connection refused",
		},
		{
			name:      "an update reporting misbehavior is not submitted",
			updates:   3,
			submitter: &recordingTxSubmitter{misbehavior: map[byte]bool{2: true}},
			wantLog:   []string{"submit 0x1 updates=[1] packets=0", "mined 0x1"},
			wantErr:   "client update 2 of 3: a client update would report misbehavior",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				payloads := make([][]byte, tc.updates)
				for i := range payloads {
					payloads[i] = []byte{byte(i + 1)}
				}

				chainClient := mocks.NewMockClient(t)
				chainClient.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
				chainClient.EXPECT().ChainID().Return("8453").Once()

				mockProver := mocks.NewMockProver(t)
				mockProver.EXPECT().ClientUpdatePayloads(mock.Anything, uint64(100)).Return(payloads, nil).Once()
				mockProver.EXPECT().
					PacketProofs(mock.Anything, uint64(100), v2.ProofKindPacketCommitment, mock.Anything,
						[]channeltypesv2.Acknowledgement(nil)).
					Return([][]byte{{0xff}}, nil).
					Once()

				var logs bytes.Buffer
				logger := slog.New(slog.NewTextHandler(&logs, nil))
				submission, err := relayPackets(
					t.Context(), logger, chainClient, mockProver, byteTxBuilder{}, tc.submitter,
					"client-0", v2.RelayKindRecv, 100,
					[]v2.PacketEvent{{Height: 90, Packet: channeltypesv2.Packet{Sequence: 1}}},
				)

				require.Equal(t, tc.wantLog, tc.submitter.log)
				rest := logs.String()
				for _, fragment := range tc.wantLines {
					_, after, found := strings.Cut(rest, fragment)
					require.True(t, found, "log %q not found in order in:\n%s", fragment, logs.String())
					rest = after
				}
				if tc.wantErr != "" {
					require.ErrorContains(t, err, tc.wantErr)
					require.Nil(t, submission)
					return
				}
				require.NoError(t, err)
				require.Equal(t, fmt.Sprintf("0x%d", tc.submitter.submitted), submission.TxHash)
			})
		})
	}
}
