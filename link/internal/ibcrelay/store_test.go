package ibcrelay

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/cosmos/ibc/link/cmd/relayercmd"
)

func TestStoreTerminalImmutabilityAndPacketID(t *testing.T) {
	st := openTestStore(t)
	ctx := t.Context()

	id := relayercmd.RoutePacketID("r1", 1)
	if id != "r1-1" {
		t.Fatalf("RoutePacketID = %q, want r1-1", id)
	}
	packet := testPacket(1, 0)
	p := storedPacket{
		PacketID: id, RouteID: "r1", Packet: packet, SourceTxHash: "0xsrc",
	}
	if err := st.InsertPending(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertPending(ctx, p); err != nil {
		t.Fatal(err)
	}
	rows, err := st.Packets(ctx, id)
	if err != nil || len(rows) != 1 {
		t.Fatalf("after ON CONFLICT: %d %v", len(rows), err)
	}
	if rows[0].State != relayercmd.PacketPending {
		t.Fatalf("state = %q", rows[0].State)
	}
	if !reflect.DeepEqual(rows[0].Packet, packet) {
		t.Fatalf("packet round-trip mismatch: got %+v, want %+v", rows[0].Packet, packet)
	}

	if err = st.MarkReceived(ctx, id, testRecvTx, "0x01"); err != nil {
		t.Fatal(err)
	}
	rows, err = st.Packets(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].State != relayercmd.PacketPending || rows[0].RecvTxHash != testRecvTx || rows[0].AckHex != "0x01" {
		t.Fatalf("after MarkReceived: %+v", rows[0])
	}
	if rows[0].effectTxHash() != testRecvTx {
		t.Fatalf("pending effect = %q", rows[0].effectTxHash())
	}

	if err = st.MarkComplete(ctx, id, "0xack"); err != nil {
		t.Fatal(err)
	}
	if err = st.MarkErrorAck(ctx, id, "0xlater", "nope"); err != nil {
		t.Fatal(err)
	}
	if err = st.MarkTimedOut(ctx, id, "0xtimeout", "nope"); err != nil {
		t.Fatal(err)
	}
	if err = st.MarkComplete(ctx, id, "0xack2"); err != nil {
		t.Fatal(err)
	}
	rows, err = st.Packets(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].State != relayercmd.PacketComplete || rows[0].AckTxHash != "0xack" {
		t.Fatalf("terminal mutability broken: %+v", rows[0])
	}
}

func TestStoreOpenRejectsInMemory(t *testing.T) {
	if _, err := openStore(":memory:"); err == nil {
		t.Fatal("expected error for :memory:")
	}
}

func TestStoreSelfInitializes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.db")
	st, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	st, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	pending, err := st.PendingPackets(t.Context())
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending = %d %v", len(pending), err)
	}
}
