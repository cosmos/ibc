package stub

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/cmd/configcmd"
	"github.com/cosmos/ibc/link/cmd/relayercmd"
)

const (
	testChainA = "chain-a"
	testRouteA = "r-a"
)

func TestRelayUsesStoredPacketWithoutDiscovery(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	const canonicalHash = "ABCDEF"
	if err := st.InsertPending(ctx, storedPacket{
		PacketID: "r-a-ift-1", RouteID: testRouteA, AppType: relayercmd.AppTypeIFT,
		Sequence: 1, SourceTxHash: canonicalHash,
	}); err != nil {
		t.Fatal(err)
	}

	calls := 0
	h := statusHandler(st, testConfig(), func(context.Context, string, string) error {
		calls++
		return errors.New("must not run")
	})
	rr := relayRequest(t, h, relayercmd.RelayRequest{SourceChainID: testChainA, SourceTxHash: "abcdef"})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if calls != 0 {
		t.Fatalf("discovery calls = %d, want 0", calls)
	}
	requests, err := st.RelayRequests(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !requests[relayRequestKey{SourceChainID: testChainA, SourceTxHash: canonicalHash}] {
		t.Fatalf("relay requests = %+v, want canonical stored hash", requests)
	}
}

func TestStatusFiltersByRouteAndPacket(t *testing.T) {
	st := openTestStore(t)
	ctx := t.Context()
	for _, packet := range []storedPacket{
		{PacketID: "packet-a", RouteID: "route-a", AppType: relayercmd.AppTypeIFT, Sequence: 1},
		{PacketID: "packet-b", RouteID: "route-b", AppType: relayercmd.AppTypeGMP, Sequence: 2},
	} {
		require.NoError(t, st.InsertPending(ctx, packet))
	}
	h := statusHandler(st, testConfig(), func(context.Context, string, string) error { return nil })

	assertPackets := func(target string, want string) {
		t.Helper()
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		require.Equal(t, http.StatusOK, recorder.Code)
		var status relayercmd.Status
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &status))
		require.Len(t, status.Packets, 1)
		require.Equal(t, want, status.Packets[0].PacketID)
	}

	assertPackets(relayercmd.StatusPath+"?"+relayercmd.StatusQueryRoute+"=route-a", "packet-a")
	assertPackets(relayercmd.StatusPath+"?"+relayercmd.StatusQueryPacket+"=packet-b", "packet-b")
}

func TestRelayDiscoversOnlyNamedSourceTransaction(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	const txHash = "0xabc"
	if err := st.InsertPending(ctx, storedPacket{
		PacketID: "r-b-gmp-1", RouteID: "r-b", AppType: relayercmd.AppTypeGMP,
		Sequence: 1, SourceTxHash: txHash,
	}); err != nil {
		t.Fatal(err)
	}

	var gotChain, gotHash string
	h := statusHandler(st, testConfig(), func(ctx context.Context, chainID, sourceTxHash string) error {
		gotChain, gotHash = chainID, sourceTxHash
		return st.InsertPending(ctx, storedPacket{
			PacketID: "r-a-ift-2", RouteID: testRouteA, AppType: relayercmd.AppTypeIFT,
			Sequence: 2, SourceTxHash: txHash,
		})
	})
	rr := relayRequest(t, h, relayercmd.RelayRequest{SourceChainID: testChainA, SourceTxHash: txHash})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if gotChain != testChainA || gotHash != txHash {
		t.Fatalf("discovery got %q/%q", gotChain, gotHash)
	}
	var result relayercmd.RelayResult
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.PacketIDs) != 1 || result.PacketIDs[0] != "r-a-ift-2" {
		t.Fatalf("packet ids = %v", result.PacketIDs)
	}
}

func TestRelayDiscoveryMissAndFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code int
	}{
		{name: "miss", code: http.StatusNotFound},
		{name: "failure", err: errors.New("rpc unavailable"), code: http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := openTestStore(t)
			h := statusHandler(st, testConfig(), func(context.Context, string, string) error { return tc.err })
			rr := relayRequest(t, h, relayercmd.RelayRequest{SourceChainID: testChainA, SourceTxHash: "0xmissing"})
			if rr.Code != tc.code {
				t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func openTestStore(t *testing.T) *stubStore {
	t.Helper()
	st, err := openStore(filepath.Join(t.TempDir(), "relayer.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func testConfig() *configcmd.Config {
	return &configcmd.Config{Relayer: configcmd.Relayer{Routes: []configcmd.Route{
		{ID: testRouteA, Source: testChainA, Destination: "chain-b"},
		{ID: "r-b", Source: "chain-b", Destination: testChainA},
	}}}
}

func relayRequest(t *testing.T, h http.Handler, req relayercmd.RelayRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, relayercmd.RelayPath, strings.NewReader(string(body))))
	return rr
}
