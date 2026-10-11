// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"errors"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/store"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

func TestRetryPacket(t *testing.T) {
	hash := common.HexToHash("0x12").Hex()
	rpcError := errors.New("RPC unavailable")
	for _, operation := range []string{"recv", "ack", "timeout"} {
		for _, observation := range []struct {
			name  string
			retry bool
			err   error
		}{
			{"mined", false, nil}, {"retry", true, nil}, {"pending", false, v2.ErrTxNotFound}, {"unavailable", false, rpcError},
		} {
			t.Run(operation+"/"+observation.name, func(t *testing.T) {
				source, destination := mocks.NewMockTxSubmitter(t), mocks.NewMockTxSubmitter(t)
				submitter := source
				db := mocks.NewMockRepository(t)
				sentAt := time.Now()
				tr := NewTransfer(store.Packet{
					SourceChainID: "1", DestinationChainID: "2",
				}, slog.Default())
				var recorded **string
				var clearMethod string
				switch operation {
				case "recv":
					submitter, recorded, clearMethod = destination, &tr.RecvTxHash, "ClearPacketRecvTx"
					tr.RecvTxTime = &sentAt
					tr.WriteAckTxHash = &hash
					tr.WriteAckTxFinalizedTime = &sentAt
				case "ack":
					recorded, clearMethod = &tr.AckTxHash, "ClearPacketAckTx"
					tr.AckTxTime = &sentAt
				case "timeout":
					recorded, clearMethod = &tr.TimeoutTxHash, "ClearPacketTimeoutTx"
					tr.TimeoutTxTime = &sentAt
				}
				*recorded = &hash
				submitter.EXPECT().ShouldRetry(mock.Anything, hash, sentAt).Return(observation.retry, observation.err)
				if observation.retry {
					db.On(clearMethod, mock.Anything, tr.Key()).Return(nil).Once()
				}
				p := NewRetryPacket(source, destination, db)
				_, err := p.Process(t.Context(), tr)
				switch {
				case errors.Is(observation.err, rpcError):
					require.ErrorIs(t, err, rpcError)
				case observation.retry || errors.Is(observation.err, v2.ErrTxNotFound):
					require.ErrorIs(t, err, ErrSubmissionPending)
				default:
					require.NoError(t, err)
				}
				if observation.retry {
					require.Nil(t, *recorded)
					if operation == "recv" {
						require.Nil(t, tr.WriteAckTxHash)
						require.Nil(t, tr.WriteAckTxFinalizedTime)
					}
				} else {
					require.Equal(t, &hash, *recorded)
				}
			})
		}
	}
}

func TestCheckSettlementFinality(t *testing.T) {
	hash := common.HexToHash("0x12").Hex()
	for _, operation := range []string{"ack", "timeout"} {
		for _, finalized := range []uint64{9, 10, 11} {
			t.Run(operation+"/"+strconv.FormatUint(finalized, 10), func(t *testing.T) {
				client := mocks.NewMockClient(t)
				tr := NewTransfer(store.Packet{SourceChainID: "1"}, slog.Default())
				if operation == "ack" {
					tr.AckTxHash = &hash
				} else {
					tr.TimeoutTxHash = &hash
				}
				client.EXPECT().TxHeight(mock.Anything, common.HexToHash(hash).Bytes()).Return(uint64(10), nil)
				proof := mocks.NewMockProver(t)
				proof.EXPECT().LatestProvableHeight(mock.Anything).Return(finalized, time.Now(), nil)
				p := CheckSettlementFinality{client: client, prover: proof}
				_, err := p.Process(t.Context(), tr)
				if finalized < 10 {
					require.ErrorIs(t, err, ErrSubmissionPending)
				} else {
					require.NoError(t, err)
				}
			})
		}
	}
}
