// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	proto "github.com/cosmos/ibc/link/api/v2/relayer"
	relayerservice "github.com/cosmos/ibc/link/internal/service/relayer"
)

type relayerServiceStub struct {
	relay  func(relayerservice.RelayRequest) error
	status []relayerservice.PacketStatus
}

func (s *relayerServiceStub) Relay(_ context.Context, request relayerservice.RelayRequest) error {
	return s.relay(request)
}

func (s *relayerServiceStub) Packets(
	_ context.Context,
	_ relayerservice.PacketFilter,
	_ relayerservice.PacketQuery,
) (relayerservice.PacketPage, error) {
	return relayerservice.PacketPage{Packets: s.status}, nil
}

func TestRelayerHandlerRelaySelection(t *testing.T) {
	t.Run("all", func(t *testing.T) {
		handler := NewRelayerHandler(&relayerServiceStub{relay: func(request relayerservice.RelayRequest) error {
			assert.Equal(t, relayerservice.RelayRequest{
				ChainID: "1", TxHash: "0xabc", Selection: relayerservice.SelectionAll,
			}, request)
			return nil
		}})

		_, err := handler.Relay(context.Background(), connect.NewRequest(&proto.RelayRequest{
			SourceChainId: "1",
			TxHash:        "0xabc",
			Selection: &proto.RelayRequest_AllPackets{
				AllPackets: &proto.AllPackets{},
			},
		}))
		require.NoError(t, err)
	})

	t.Run("selected", func(t *testing.T) {
		handler := NewRelayerHandler(&relayerServiceStub{relay: func(request relayerservice.RelayRequest) error {
			assert.Equal(t, relayerservice.SelectionExplicit, request.Selection)
			assert.Equal(t, []relayerservice.PacketSelector{
				{SourceClientID: "base-0", SequenceNumber: 2},
				{SourceClientID: "base-0", SequenceNumber: 6},
			}, request.Packets)
			return nil
		}})

		_, err := handler.Relay(context.Background(), connect.NewRequest(&proto.RelayRequest{
			Selection: &proto.RelayRequest_SelectedPackets{SelectedPackets: &proto.SelectedPackets{
				Packets: []*proto.PacketSelector{
					{SourceClientId: "base-0", SequenceNumber: 2},
					{SourceClientId: "base-0", SequenceNumber: 6},
				},
			}},
		}))
		require.NoError(t, err)
	})

	t.Run("failedPrecondition", func(t *testing.T) {
		handler := NewRelayerHandler(&relayerServiceStub{relay: func(relayerservice.RelayRequest) error {
			return relayerservice.ErrFailedPrecondition
		}})
		_, err := handler.Relay(context.Background(), connect.NewRequest(&proto.RelayRequest{}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	})
}

func TestRelayerHandlerPacketsMapsNotSelected(t *testing.T) {
	handler := NewRelayerHandler(&relayerServiceStub{
		status: []relayerservice.PacketStatus{{State: relayerservice.StateNotSelected}},
	})

	response, err := handler.Packets(context.Background(), connect.NewRequest(&proto.PacketsRequest{}))
	require.NoError(t, err)
	require.Len(t, response.Msg.GetPackets(), 1)
	assert.Equal(t, proto.PacketState_PACKET_STATE_NOT_SELECTED, response.Msg.GetPackets()[0].GetState())
}

// An unspecified state names no constraint, but a state this build does not
// know must be rejected: folding it into unspecified would silently return the
// unfiltered listing to a caller that asked for a narrow one.
func TestRelayerHandlerPacketStateFilter(t *testing.T) {
	for _, tt := range []struct {
		name    string
		state   proto.PacketState
		wantErr bool
	}{
		{"unspecified matches everything", proto.PacketState_PACKET_STATE_UNSPECIFIED, false},
		{"known state", proto.PacketState_PACKET_STATE_SUCCEEDED, false},
		{"unknown state is rejected", proto.PacketState(9999), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewRelayerHandler(&relayerServiceStub{})

			_, err := handler.Packets(context.Background(), connect.NewRequest(&proto.PacketsRequest{
				Filter: &proto.PacketFilter{State: tt.state},
			}))

			if !tt.wantErr {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		})
	}
}
