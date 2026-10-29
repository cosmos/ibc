// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/cli/internal/chains/evm"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

func TestRelayPacketsOnIdleChain(t *testing.T) {
	for _, kind := range []v2.RelayKind{v2.RelayKindAck, v2.RelayKindRecv, v2.RelayKindTimeout} {
		t.Run(kind.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()

			eth := mocks.NewMockETHClient(t)
			// The chain produces no blocks until a transaction is submitted.
			eth.EXPECT().HeaderByNumber(mock.Anything, (*big.Int)(nil)).Return(&types.Header{
				Number: big.NewInt(100), Time: 1,
			}, nil).Maybe()
			chain, err := evm.NewWithClient("1", eth, "0x0000000000000000000000000000000000000001",
				evm.WithWaitForChain(false))
			require.NoError(t, err)

			packet := channeltypesv2.Packet{Sequence: 42, SourceClient: "client-0"}
			event := v2.PacketEvent{Packet: packet, Height: 100}
			var acks []channeltypesv2.Acknowledgement
			if kind == v2.RelayKindAck {
				event.Acks = [][]byte{{0x01}}
				acks = []channeltypesv2.Acknowledgement{channeltypesv2.NewAcknowledgement(event.Acks...)}
			}

			proof := []byte{0x02}
			prover := mocks.NewMockProver(t)
			prover.EXPECT().ClientUpdatePayloads(mock.Anything, uint64(100)).Return([][]byte{{0x03}}, nil).Once()
			prover.EXPECT().PacketProofs(mock.Anything, uint64(100), proofKindFor(kind),
				[]channeltypesv2.Packet{packet}, acks).Return([][]byte{proof}, nil).Once()

			relayTx := v2.RelayTx{To: common.HexToAddress("0x01").Bytes(), Data: []byte{0x04}}
			builder := mocks.NewMockTxBuilder(t)
			builder.EXPECT().BuildRelayTxs(v2.ClientUpdate{ClientID: "client-0", Payloads: [][]byte{{0x03}}},
				[]v2.PacketRelayItem{{Kind: kind, Packet: packet, Acks: event.Acks, Proof: proof, ProofHeight: 100}}).
				Return([]v2.RelayTx{relayTx}, nil).Once()

			submission := &v2.Submission{TxHash: "0xrelay"}
			submitter := mocks.NewMockTxSubmitter(t)
			simulation := submitter.EXPECT().SimulateRelayTx(mock.Anything, relayTx).Return(v2.RelaySimulation{
				Operations: []v2.OperationResult{{Type: v2.OperationTypePacket, Success: true}},
			}, nil).Once()
			submitter.EXPECT().Submit(mock.Anything, v2.TxIntent{
				To: common.BytesToAddress(relayTx.To).Hex(), Data: relayTx.Data,
			}).Return(submission, nil).Once().NotBefore(simulation)

			got, err := relayPackets(ctx, slog.Default(), chain, prover, builder, submitter,
				"client-0", kind, 100, []v2.PacketEvent{event})
			require.NoError(t, err)
			require.Equal(t, submission, got)
		})
	}
}
