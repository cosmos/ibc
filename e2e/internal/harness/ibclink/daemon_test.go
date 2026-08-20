// SPDX-License-Identifier: Apache-2.0

package ibclink

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

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

func (f *fakeRelayerAPI) Packets(
	_ context.Context,
	req *connect.Request[relayerv2.PacketsRequest],
) (*connect.Response[relayerv2.PacketsResponse], error) {
	filter := req.Msg.GetFilter()
	// An unknown transaction lists nothing
	packets := f.statuses[filter.GetSourceChainId()+"/"+filter.GetSourceTxHash()]

	return connect.NewResponse(&relayerv2.PacketsResponse{
		Packets: packets,
	}), nil
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
		waitPolicies: map[string]WaitPolicy{
			"route-manual": {CompletionBudget: time.Second, StatusPoll: time.Millisecond, StabilityWindow: time.Second},
		},
	}
}

// The relayer explains startup failures (config rejections above all) only on
// stderr; the error must carry that tail because the log file dies with the
// test's TempDir.
func TestStartRelayerSurfacesStartupLogs(t *testing.T) {
	script := filepath.Join(t.TempDir(), "ibc-fail")
	require.NoError(t, os.WriteFile(
		script,
		[]byte("#!/bin/sh\necho '.signers[0].grpc required for remote signer' >&2\nexit 1\n"),
		0o700,
	))
	t.Setenv(binEnv, script)

	driver, err := NewDriver(filepath.Join(t.TempDir(), "ibc.yml"))
	require.NoError(t, err)
	_, err = driver.StartRelayer(t.Context())
	require.Error(t, err)
	require.ErrorContains(t, err, "no readiness line")
	require.ErrorContains(t, err, ".signers[0].grpc required for remote signer")
}

func TestRelayerProbeAcceptsEmptyPacketListing(t *testing.T) {
	relayer := testRelayer(t, &fakeRelayerAPI{})
	require.NoError(t, relayer.probePacketsEndpoint(context.Background()))
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

	// An unindexed transaction lists nothing; it is not an error.
	unknown, err := relayer.PacketStatuses(ctx, "chain-a", "0xother")
	require.NoError(t, err)
	require.Empty(t, unknown)

	_, err = relayer.PacketStatuses(ctx, "chain-c", "0xabc")
	require.Error(t, err)
	require.ErrorContains(t, err, "no configured chain id")

	require.NoError(t, relayer.RelayAll(ctx, "chain-b", "0xdef"))
	require.Len(t, api.relayed, 1)
	require.Equal(t, "31338", api.relayed[0].SourceChainId)
	require.Equal(t, "0xdef", api.relayed[0].TxHash)
	require.NotNil(t, api.relayed[0].GetAllPackets())

	require.NoError(t, relayer.RelaySelected(ctx, "chain-a", "0x123", &relayerv2.PacketSelector{
		SourceClientId: "client-0",
		SequenceNumber: 7,
	}))
	require.Len(t, api.relayed, 2)
	require.Equal(t, "31337", api.relayed[1].SourceChainId)
	require.Equal(t, "0x123", api.relayed[1].TxHash)
	require.Equal(t, []*relayerv2.PacketSelector{{SourceClientId: "client-0", SequenceNumber: 7}},
		api.relayed[1].GetSelectedPackets().GetPackets())

	require.True(t, relayer.ManualRoute("route-manual"))
	require.False(t, relayer.ManualRoute("route-auto"))
	policy, ok := relayer.WaitPolicy("route-manual")
	require.True(t, ok)
	require.Equal(t, time.Second, policy.CompletionBudget)
	_, ok = relayer.WaitPolicy("route-auto")
	require.False(t, ok)
}

func TestWaitPoliciesSurviveDriverAndRelayerStartup(t *testing.T) {
	api := &fakeRelayerAPI{}
	mux := http.NewServeMux()
	path, handler := relayerv2.NewRelayerApiServiceHandler(api)
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	script := filepath.Join(t.TempDir(), "ibc-ready")
	require.NoError(t, os.WriteFile(script, []byte(fmt.Sprintf(
		"#!/bin/sh\nprintf '%%s\\n' '%s'\nexec sleep 60\n",
		fmt.Sprintf(`{"event":"ready","http":%q}`, strings.TrimPrefix(server.URL, "http://")),
	)), 0o700))
	t.Setenv(binEnv, script)

	driver, err := NewDriver(filepath.Join(t.TempDir(), "ibc.yml"))
	require.NoError(t, err)
	want := WaitPolicy{
		CompletionBudget: 17 * time.Second,
		StatusPoll:       23 * time.Millisecond,
		StabilityWindow:  5 * time.Second,
	}
	require.NoError(t, driver.ConfigureRelayer(RelayerOptions{
		WaitPolicies: map[string]WaitPolicy{"route-a": want},
	}))
	relayer, err := driver.StartRelayer(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, relayer.Stop(context.Background())) })

	got, ok := relayer.WaitPolicy("route-a")
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestConfigureRelayerRejectsInvalidWaitPolicy(t *testing.T) {
	driver, err := NewDriver(filepath.Join(t.TempDir(), "ibc.yml"))
	require.NoError(t, err)
	err = driver.ConfigureRelayer(RelayerOptions{
		WaitPolicies: map[string]WaitPolicy{"route-a": {}},
	})
	require.ErrorContains(t, err, `route "route-a" has invalid wait policy`)
}
