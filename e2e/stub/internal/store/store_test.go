package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/cosmos/ibc/e2e/internal/harness/ibclink/wire"
)

const testSourceTxHash = "0xsrc"

func packetByID(ctx context.Context, s *Store, packetID string) (Packet, bool, error) {
	rows, err := s.Packets(ctx, packetID)
	if err != nil || len(rows) == 0 {
		return Packet{}, false, err
	}
	return rows[0], true, nil
}

func open(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "relayer.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestOpenRejectsInMemory(t *testing.T) {
	_, err := Open(":memory:")
	if err == nil {
		t.Fatal("Open(:memory:) succeeded; want error")
	}
}

func TestOpenSelfInitializesSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relayer.db")
	ctx := context.Background()

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	_, found, loadErr := st.LoadTestApps(ctx)
	if loadErr != nil || found {
		t.Fatalf("load on self-initialized empty store = found %v, err %v; want not found", found, loadErr)
	}
	if closeErr := st.Close(); closeErr != nil {
		t.Fatalf("close first store: %v", closeErr)
	}

	st, err = Open(path)
	if err != nil {
		t.Fatalf("open store again: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, found, err := st.LoadTestApps(ctx); err != nil || found {
		t.Fatalf("load after second open = found %v, err %v; want not found", found, err)
	}
}

func TestOpenCoexistsWithRealLinkTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relayer.db")
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		t.Fatalf("open real Link database: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE packets (id INTEGER PRIMARY KEY);
		CREATE TABLE relay_requests (id INTEGER PRIMARY KEY);
		CREATE TABLE test_app_deployments (id INTEGER PRIMARY KEY);`)
	if err != nil {
		t.Fatalf("create real Link tables: %v", err)
	}
	if closeErr := db.Close(); closeErr != nil {
		t.Fatalf("close real Link database: %v", closeErr)
	}

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open stub store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	if err := st.InsertPending(ctx, Packet{PacketID: "packet-1", RouteID: "route-1"}); err != nil {
		t.Fatalf("insert stub packet: %v", err)
	}
	if err := st.RequestRelay(ctx, "chain-a", "0xsource"); err != nil {
		t.Fatalf("insert stub relay request: %v", err)
	}
	if _, found, err := st.LoadTestApps(ctx); err != nil || found {
		t.Fatalf("load stub test apps = found %v, err %v; want not found", found, err)
	}
}

func TestTestAppDeploymentRoundTrip(t *testing.T) {
	st := open(t)
	ctx := context.Background()

	if _, found, err := st.LoadTestApps(ctx); err != nil || found {
		t.Fatalf("load on empty = found %v, err %v; want not found", found, err)
	}

	want := wire.TestAppDeployment{
		Chains: map[string]wire.ChainTestAppDeployment{
			"31337": {
				MockIFT: "0xaaa", MockGMP: "0xbbb", Counter: "0xccc", TxHash: "0xreceipt",
			},
		},
	}
	if saveErr := st.SaveTestApps(ctx, want); saveErr != nil {
		t.Fatalf("save: %v", saveErr)
	}
	got, found, err := st.LoadTestApps(ctx)
	if err != nil || !found {
		t.Fatalf("load = found %v, err %v; want found", found, err)
	}
	if got.Chains["31337"].MockIFT != "0xaaa" || got.Chains["31337"].TxHash != "0xreceipt" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	want.Chains["31337"] = wire.ChainTestAppDeployment{MockIFT: "0xnew"}
	if saveErr := st.SaveTestApps(ctx, want); saveErr != nil {
		t.Fatalf("re-save: %v", saveErr)
	}
	got, found, err = st.LoadTestApps(ctx)
	if err != nil || !found {
		t.Fatalf("load after re-save = found %v, err %v; want found", found, err)
	}
	if got.Chains["31337"].MockIFT != "0xnew" {
		t.Fatalf("upsert did not overwrite: %+v", got)
	}
}

func TestPacketLifecycle(t *testing.T) {
	st := open(t)
	ctx := context.Background()

	id := "r1-ift-1"
	p := Packet{
		PacketID:         id,
		RouteID:          "r1",
		AppType:          wire.AppTypeIFT,
		Sequence:         1,
		SourceTxHash:     testSourceTxHash,
		Receiver:         "0xreceiver",
		Amount:           "42",
		TimeoutTimestamp: "99",
	}
	if err := st.InsertPending(ctx, p); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := st.InsertPending(ctx, p); err != nil {
		t.Fatalf("re-insert: %v", err)
	}

	got, found, err := packetByID(ctx, st, id)
	if err != nil || !found {
		t.Fatalf("packet = found %v, err %v", found, err)
	}
	if got.State != wire.PacketPending || got.SourceTxHash != testSourceTxHash || got.AppType != wire.AppTypeIFT ||
		got.Amount != "42" {
		t.Fatalf("pending packet = %+v", got)
	}

	pending, err := st.PendingPackets(ctx)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending packets = %d, %v; want 1", len(pending), err)
	}

	if completeErr := st.MarkComplete(ctx, id, "0xrecv"); completeErr != nil {
		t.Fatalf("complete: %v", completeErr)
	}
	got, found, err = packetByID(ctx, st, id)
	if err != nil || !found {
		t.Fatalf("completed packet = found %v, err %v", found, err)
	}
	if got.State != wire.PacketComplete || got.EffectTxHash != "0xrecv" {
		t.Fatalf("completed packet = %+v", got)
	}

	if insertErr := st.InsertPending(ctx, p); insertErr != nil {
		t.Fatalf("re-insert post-complete: %v", insertErr)
	}
	got, found, err = packetByID(ctx, st, id)
	if err != nil || !found {
		t.Fatalf("packet after re-insert = found %v, err %v", found, err)
	}
	if got.State != wire.PacketComplete {
		t.Fatalf("re-insert regressed state to %q", got.State)
	}

	all, err := st.Packets(ctx, "")
	if err != nil || len(all) != 1 {
		t.Fatalf("all packets = %d, %v; want 1", len(all), err)
	}
	if byPacket, queryErr := st.Packets(ctx, id); queryErr != nil || len(byPacket) != 1 {
		t.Fatalf("by packet = %d, %v; want 1", len(byPacket), queryErr)
	}
	if none, queryErr := st.Packets(ctx, "missing-packet"); queryErr != nil || len(none) != 0 {
		t.Fatalf("by missing packet = %d, %v; want 0", len(none), queryErr)
	}
}

func TestRelayRequestsAreIdempotent(t *testing.T) {
	st := open(t)
	ctx := context.Background()

	if err := st.RequestRelay(ctx, "chain-a", testSourceTxHash); err != nil {
		t.Fatalf("request relay: %v", err)
	}
	if err := st.RequestRelay(ctx, "chain-a", testSourceTxHash); err != nil {
		t.Fatalf("repeat request relay: %v", err)
	}

	got, err := st.RelayRequests(ctx)
	if err != nil {
		t.Fatalf("relay requests: %v", err)
	}
	if !got[(RelayRequestKey{SourceChainID: "chain-a", SourceTxHash: testSourceTxHash})] || len(got) != 1 {
		t.Fatalf("relay requests = %+v; want one chain-a/0xsrc entry", got)
	}
}

func TestPacketsBySourceTxIsCaseInsensitive(t *testing.T) {
	st := open(t)
	ctx := context.Background()
	for _, p := range []Packet{
		{PacketID: "p1", RouteID: "r1", AppType: wire.AppTypeIFT, Sequence: 1, SourceTxHash: "ABCDEF"},
		{PacketID: "p2", RouteID: "r1", AppType: wire.AppTypeGMP, Sequence: 2, SourceTxHash: "0xother"},
	} {
		if err := st.InsertPending(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	got, err := st.PacketsBySourceTx(ctx, "abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].PacketID != "p1" || got[0].SourceTxHash != "ABCDEF" {
		t.Fatalf("packets = %+v", got)
	}
}

// Terminal Mark* writers must not regress complete/timed_out/error_ack (enforced by notTerminalClause).
func TestTerminalStatesAreImmutable(t *testing.T) {
	st := open(t)
	ctx := context.Background()

	seed := func(id string, mark func() error) {
		t.Helper()
		if err := st.InsertPending(
			ctx,
			Packet{PacketID: id, RouteID: "r1", AppType: wire.AppTypeIFT, Sequence: 1},
		); err != nil {
			t.Fatalf("seed insert %s: %v", id, err)
		}
		if err := mark(); err != nil {
			t.Fatalf("seed mark %s: %v", id, err)
		}
	}
	stateOf := func(id string) wire.PacketState {
		t.Helper()
		p, found, err := packetByID(ctx, st, id)
		if err != nil || !found {
			t.Fatalf("read %s: found %v err %v", id, found, err)
		}
		return p.State
	}

	timedOut := wire.PacketID("r1", wire.AppTypeIFT, 1)
	seed(timedOut, func() error { return st.MarkTimedOut(ctx, timedOut, "0xrefund", "deadline") })
	if err := st.MarkErrorAck(ctx, timedOut, "0xack", "late ack"); err != nil {
		t.Fatalf("error-ack a timed_out packet: %v", err)
	}
	if err := st.MarkComplete(ctx, timedOut, "0xrecv"); err != nil {
		t.Fatalf("complete a timed_out packet: %v", err)
	}
	if got := stateOf(timedOut); got != wire.PacketTimedOut {
		t.Fatalf("timed_out packet regressed to %q", got)
	}

	errAck := wire.PacketID("r1", wire.AppTypeGMP, 1)
	seed(errAck, func() error { return st.MarkErrorAck(ctx, errAck, "0xdeliver", "inner revert") })
	if err := st.MarkTimedOut(ctx, errAck, "0xrefund", "late deadline"); err != nil {
		t.Fatalf("time-out an error_ack packet: %v", err)
	}
	if err := st.MarkComplete(ctx, errAck, "0xrecv"); err != nil {
		t.Fatalf("complete an error_ack packet: %v", err)
	}
	if got := stateOf(errAck); got != wire.PacketErrorAck {
		t.Fatalf("error_ack packet regressed to %q", got)
	}

	done := wire.PacketID("r2", wire.AppTypeIFT, 1)
	seed(done, func() error { return st.MarkComplete(ctx, done, "0xrecv1") })
	if err := st.MarkComplete(ctx, done, "0xrecv2"); err != nil {
		t.Fatalf("re-complete a complete packet: %v", err)
	}
	if got := stateOf(done); got != wire.PacketComplete {
		t.Fatalf("complete packet changed state to %q", got)
	}
	pkts, err := st.Packets(ctx, done)
	if err != nil {
		t.Fatalf("read re-completed packet: %v", err)
	}
	if len(pkts) != 1 || pkts[0].EffectTxHash != "0xrecv1" {
		t.Fatalf("re-complete rewrote the terminal effect hash: %+v", pkts)
	}
}
