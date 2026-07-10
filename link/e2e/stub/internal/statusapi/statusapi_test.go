package statusapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/e2e/stub/internal/store"
)

func TestRelayUsesStoredPacketWithoutDiscovery(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	const canonicalHash = "ABCDEF"
	if err := st.InsertPending(ctx, store.Packet{
		PacketID: "r-a-ift-1", RouteID: "r-a", AppType: wire.AppTypeIFT,
		Sequence: 1, SourceTxHash: canonicalHash,
	}); err != nil {
		t.Fatal(err)
	}

	calls := 0
	h := Handler(st, testConfig(), func(context.Context, string, string) error {
		calls++
		return errors.New("must not run")
	})
	rr := relayRequest(t, h, wire.RelayRequest{SourceChainID: "chain-a", SourceTxHash: "abcdef"})
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
	if !requests[store.RelayRequestKey{SourceChainID: "chain-a", SourceTxHash: canonicalHash}] {
		t.Fatalf("relay requests = %+v, want canonical stored hash", requests)
	}
}

func TestRelayDiscoversOnlyNamedSourceTransaction(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	const txHash = "0xabc"
	if err := st.InsertPending(ctx, store.Packet{
		PacketID: "r-b-gmp-1", RouteID: "r-b", AppType: wire.AppTypeGMP,
		Sequence: 1, SourceTxHash: txHash,
	}); err != nil {
		t.Fatal(err)
	}

	var gotChain, gotHash string
	h := Handler(st, testConfig(), func(ctx context.Context, chainID, sourceTxHash string) error {
		gotChain, gotHash = chainID, sourceTxHash
		return st.InsertPending(ctx, store.Packet{
			PacketID: "r-a-ift-2", RouteID: "r-a", AppType: wire.AppTypeIFT,
			Sequence: 2, SourceTxHash: txHash,
		})
	})
	rr := relayRequest(t, h, wire.RelayRequest{SourceChainID: "chain-a", SourceTxHash: txHash})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if gotChain != "chain-a" || gotHash != txHash {
		t.Fatalf("discovery got %q/%q", gotChain, gotHash)
	}
	var result wire.RelayResult
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
			h := Handler(st, testConfig(), func(context.Context, string, string) error { return tc.err })
			rr := relayRequest(t, h, wire.RelayRequest{SourceChainID: "chain-a", SourceTxHash: "0xmissing"})
			if rr.Code != tc.code {
				t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "relayer.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func testConfig() *wire.ConfigYAML {
	return &wire.ConfigYAML{Relayer: wire.Relayer{Routes: []wire.Route{
		{ID: "r-a", Source: "chain-a", Destination: "chain-b"},
		{ID: "r-b", Source: "chain-b", Destination: "chain-a"},
	}}}
}

func relayRequest(t *testing.T, h http.Handler, req wire.RelayRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, wire.RelayPath, strings.NewReader(string(body))))
	return rr
}
