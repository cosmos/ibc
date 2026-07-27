package ibclink

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/cmd/relayercmd"

	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

func TestParseReadiness(t *testing.T) {
	valid := `{"event":"ready","chainsConnected":["chain-a"],"http":"127.0.0.1:4242"}` + "\n"
	res := parseReadiness(valid)
	require.NoError(t, res.err)
	require.Equal(t, "127.0.0.1:4242", res.readiness.HTTP)
	require.Equal(t, []string{"chain-a"}, res.readiness.ChainsConnected)

	res = parseReadiness("plain log line, not json\n")
	require.Error(t, res.err)
	require.ErrorContains(t, res.err, "not readiness JSON")

	wrongEvent := `{"event":"started","http":"127.0.0.1:4242"}`
	res = parseReadiness(wrongEvent)
	require.Error(t, res.err)
	require.ErrorContains(t, res.err, "invalid readiness")

	notReady := `{"event":"ready"}`
	res = parseReadiness(notReady)
	require.Error(t, res.err)
	require.ErrorContains(t, res.err, "http")
}

// fakeRelayerAPI serves the relayer wire contract: Status on an unknown
// transaction reports CodeNotFound; Relay records the submitted arguments.
type fakeRelayerAPI struct {
	relayerv2.UnimplementedRelayerApiServiceHandler
	relayed  []*relayerv2.RelayRequest
	statuses map[string][]*relayerv2.PacketStatus
}

func (f *fakeRelayerAPI) Relay(
	_ context.Context,
	req *connect.Request[relayerv2.RelayRequest],
) (*connect.Response[relayerv2.RelayResponse], error) {
	f.relayed = append(f.relayed, req.Msg)
	return connect.NewResponse(&relayerv2.RelayResponse{}), nil
}

func (f *fakeRelayerAPI) Status(
	_ context.Context,
	req *connect.Request[relayerv2.StatusRequest],
) (*connect.Response[relayerv2.StatusResponse], error) {
	statuses, ok := f.statuses[req.Msg.ChainId+"/"+req.Msg.TxHash]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("transaction not submitted to relayer"))
	}
	return connect.NewResponse(&relayerv2.StatusResponse{PacketStatuses: statuses}), nil
}

func testRelayer(t *testing.T, api *fakeRelayerAPI) *Relayer {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := relayerv2.NewRelayerApiServiceHandler(api)
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return &Relayer{
		client: relayerv2.NewRelayerApiServiceClient(server.Client(), server.URL, connect.WithGRPC()),
		chainIDs: map[string]string{
			"chain-a": "31337",
			"chain-b": "31338",
		},
		manualRoutes: map[string]bool{"route-manual": true},
	}
}

func TestRelayerProbeAcceptsNotFoundStatus(t *testing.T) {
	relayer := testRelayer(t, &fakeRelayerAPI{})
	require.NoError(t, relayer.probeStatusEndpoint(context.Background()))
}

func TestRelayerTranslatesChainIDs(t *testing.T) {
	api := &fakeRelayerAPI{statuses: map[string][]*relayerv2.PacketStatus{
		"31337/0xabc": {{SequenceNumber: 7}},
	}}
	relayer := testRelayer(t, api)
	ctx := context.Background()

	statuses, err := relayer.PacketStatuses(ctx, "chain-a", "0xabc")
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	require.Equal(t, uint64(7), statuses[0].SequenceNumber)

	_, err = relayer.PacketStatuses(ctx, "chain-a", "0xother")
	require.Error(t, err)
	require.True(t, IsStatusNotFound(err))

	_, err = relayer.PacketStatuses(ctx, "chain-c", "0xabc")
	require.Error(t, err)
	require.False(t, IsStatusNotFound(err))
	require.ErrorContains(t, err, "no configured chain id")

	require.NoError(t, relayer.Relay(ctx, relayercmd.RelayRequest{
		SourceChainID: "chain-b",
		SourceTxHash:  "0xdef",
	}))
	require.Len(t, api.relayed, 1)
	require.Equal(t, "31338", api.relayed[0].ChainId)
	require.Equal(t, "0xdef", api.relayed[0].TxHash)

	require.True(t, relayer.ManualRoute("route-manual"))
	require.False(t, relayer.ManualRoute("route-auto"))
}
