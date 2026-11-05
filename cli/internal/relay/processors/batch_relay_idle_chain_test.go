// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"fmt"
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
		for _, updateCount := range []int{0, 1, 3} {
			t.Run(fmt.Sprintf("%s/%d-updates", kind, updateCount), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()

				eth := mocks.NewMockETHClient(t)
				// The head stays frozen until a relay transaction is submitted.
				eth.EXPECT().HeaderByNumber(mock.Anything, (*big.Int)(nil)).Return(&types.Header{
					Number: big.NewInt(100), Time: 1,
				}, nil).Maybe()
				chain, err := evm.NewWithClient("324", eth, "0x0000000000000000000000000000000000000001",
					evm.WithWaitForChain(false))
				require.NoError(t, err)

				packet := channeltypesv2.Packet{Sequence: 42, SourceClient: "client-0"}
				event := v2.PacketEvent{Packet: packet, Height: 100}
				var acks []channeltypesv2.Acknowledgement
				if kind == v2.RelayKindAck {
					event.Acks = [][]byte{{0x01}}
					acks = []channeltypesv2.Acknowledgement{channeltypesv2.NewAcknowledgement(event.Acks...)}
				}
				payloads := make([][]byte, updateCount)
				for i := range payloads {
					payloads[i] = []byte{byte(i + 1)}
				}
				prover := mocks.NewMockProver(t)
				prover.EXPECT().ClientUpdatePayloads(mock.Anything, uint64(100)).Return(payloads, nil).Once()
				prover.EXPECT().PacketProofs(mock.Anything, uint64(100), proofKindFor(kind),
					[]channeltypesv2.Packet{packet}, acks).Return([][]byte{{0xff}}, nil).Once()

				submitter := mocks.NewMockTxSubmitter(t)
				var previous *mock.Call
				for i := range max(updateCount, 1) {
					last := i == max(updateCount, 1)-1
					data := []byte{0}
					operation := v2.OperationTypeUpdate
					if last {
						data[0] = 1
						operation = v2.OperationTypePacket
					}
					if updateCount > 0 {
						data = append(data, payloads[i]...)
					}
					simulation := submitter.EXPECT().SimulateRelayTx(mock.Anything, v2.RelayTx{Data: data}).
						Return(v2.RelaySimulation{Operations: []v2.OperationResult{{Type: operation, Success: true}}}, nil).
						Once()
					if previous != nil {
						simulation.NotBefore(previous)
					}
					hash := fmt.Sprintf("0x%d", i+1)
					previous = submitter.EXPECT().Submit(mock.Anything, v2.TxIntent{
						To: common.Address{}.Hex(), Data: data,
					}).Return(&v2.Submission{TxHash: hash}, nil).Once().NotBefore(simulation)
					if !last {
						previous = submitter.EXPECT().ShouldRetry(mock.Anything, hash, mock.Anything).
							Return(false, nil).Once().NotBefore(previous)
					}
				}

				got, err := relayPackets(ctx, slog.Default(), chain, prover, byteTxBuilder{}, submitter,
					"client-0", kind, 100, []v2.PacketEvent{event})
				require.NoError(t, err)
				require.Equal(t, fmt.Sprintf("0x%d", max(updateCount, 1)), got.TxHash)
			})
		}
	}
}
