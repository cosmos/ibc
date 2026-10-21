// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"log/slog"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

func TestRelayPacketsBuildsWithSenderAfterChainReady(t *testing.T) {
	for _, tc := range []struct {
		name     string
		buildErr error
	}{
		{name: "success"},
		{name: "simulation rejected", buildErr: assert.AnError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			const sender = "0x0000000000000000000000000000000000001234"
			packet := channeltypesv2.Packet{Sequence: 1, SourceClient: "source-0", DestinationClient: "dest-0"}
			update := v2.ClientUpdate{ClientID: "dest-0", Payloads: [][]byte{{1}}}
			items := []v2.PacketRelayItem{{Kind: v2.RelayKindRecv, Packet: packet, Proof: []byte{2}, ProofHeight: 100}}
			chain := mocks.NewMockClient(t)
			wait := chain.EXPECT().WaitForChain(mock.Anything).Return(nil).Once()
			prover := mocks.NewMockProver(t)
			prover.EXPECT().ClientUpdatePayloads(ctx, uint64(100)).Return(update.Payloads, nil).Once()
			prover.EXPECT().PacketProofs(
				ctx, uint64(100), v2.ProofKindPacketCommitment, []channeltypesv2.Packet{packet}, []channeltypesv2.Acknowledgement(nil),
			).
				Return([][]byte{{2}}, nil).Once()
			builder := mocks.NewMockTxBuilder(t)
			submitter := mocks.NewMockTxSubmitter(t)
			tx := v2.RelayTx{To: common.HexToAddress("0x2222").Bytes(), Data: []byte{3}}
			want := &v2.Submission{TxHash: "0xtx", RelayerAddress: sender}
			submitter.EXPECT().Address().Return(sender).Once()
			build := builder.EXPECT().
				BuildRelayTxs(ctx, sender, update, items).
				Return([]v2.RelayTx{tx}, tc.buildErr).
				Once()
			build.NotBefore(wait)
			if tc.buildErr == nil {
				submit := submitter.EXPECT().
					Submit(ctx, v2.TxIntent{To: common.BytesToAddress(tx.To).Hex(), Data: tx.Data}).
					Return(want, nil).
					Once()
				submit.NotBefore(build)
				chain.EXPECT().ChainID().Return("1").Once()
			}
			got, err := relayPackets(
				ctx,
				slog.Default(),
				chain,
				prover,
				builder,
				submitter,
				"dest-0",
				v2.RelayKindRecv,
				100,
				[]v2.PacketEvent{{Packet: packet}},
			)
			require.ErrorIs(t, err, tc.buildErr)
			if tc.buildErr != nil {
				require.Nil(t, got)
			} else {
				require.Equal(t, want, got)
			}
		})
	}
}
