package ibcrelay

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/cosmos/ibc/link/cmd/configcmd"
	"github.com/cosmos/ibc/link/cmd/relayercmd"

	transfertypes "github.com/cosmos/ibc-go/v11/modules/apps/transfer/types"
)

const (
	testSrcChain   = "src"
	testDstChain   = "dst"
	testRouteID    = "r1"
	testSrcClient  = "client-src"
	testDstClient  = "client-dst"
	testSourceTx   = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testRecvTx     = "0xrecv"
	testSuccessAck = "0x01"
)

func openTestStore(t *testing.T) *relayStore {
	t.Helper()
	st, err := openStore(filepath.Join(t.TempDir(), "relayer.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func testPacket(seq uint64, timeout uint64) ics26router.IICS26RouterMsgsPacket {
	return ics26router.IICS26RouterMsgsPacket{
		Sequence:         seq,
		SourceClient:     testSrcClient,
		DestClient:       testDstClient,
		TimeoutTimestamp: timeout,
		Payloads: []ics26router.IICS26RouterMsgsPayload{{
			SourcePort: transfertypes.PortID,
			DestPort:   transfertypes.PortID,
			Version:    transfertypes.V1,
			Encoding:   transfertypes.EncodingABI,
			Value:      []byte{0xab},
		}},
	}
}

func insertPendingPacket(t *testing.T, st *relayStore, seq uint64, timeout uint64) storedPacket {
	t.Helper()
	pkt := testPacket(seq, timeout)
	p := storedPacket{
		PacketID:     relayercmd.RoutePacketID(testRouteID, seq),
		RouteID:      testRouteID,
		Packet:       pkt,
		SourceTxHash: testSourceTx,
	}
	if err := st.InsertPending(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	return p
}

func testRelayer(st *relayStore, src, dst *fakeOps, autoRelay bool) *relayer {
	var auto *configcmd.AutoRelay
	if !autoRelay {
		auto = &configcmd.AutoRelay{Enabled: false}
	}
	return &relayer{
		cfg: &configcmd.Config{
			Relayer: configcmd.Relayer{Routes: []configcmd.Route{{
				ID: testRouteID, Source: testSrcChain, Destination: testDstChain,
				Type:         configcmd.RouteEVMToEVMAttested,
				SourceClient: testSrcClient, DestClient: testDstClient,
				AutoRelay: auto,
			}}},
		},
		conns: map[string]*chainConn{
			testSrcChain: {id: testSrcChain, ops: src},
			testDstChain: {id: testDstChain, ops: dst},
		},
		store:      st,
		log:        &bytes.Buffer{},
		sentCursor: map[string]uint64{},
	}
}

func packetByID(t *testing.T, st *relayStore, id string) storedPacket {
	t.Helper()
	rows, err := st.Packets(t.Context(), id)
	if err != nil || len(rows) != 1 {
		t.Fatalf("packet %s: rows=%d err=%v", id, len(rows), err)
	}
	return rows[0]
}

func TestHappyPathRecvAckComplete(t *testing.T) {
	st := openTestStore(t)
	p := insertPendingPacket(t, st, 1, 0)
	src, dst := newFakeOps(), newFakeOps()
	dst.recvAckBySeq[1] = []byte{0x01}
	r := testRelayer(st, src, dst, true)

	if err := r.reconcilePacket(t.Context(), p, nil); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	got := packetByID(t, st, p.PacketID)
	if got.State != relayercmd.PacketComplete {
		t.Fatalf("state = %q, want complete", got.State)
	}
	if got.RecvTxHash == "" || got.AckTxHash == "" {
		t.Fatalf("missing tx hashes: %+v", got)
	}
	if got.AckHex != testSuccessAck {
		t.Fatalf("ack_hex = %q, want %q", got.AckHex, testSuccessAck)
	}
	if len(dst.recvSubmitted) != 1 || len(src.ackSubmitted) != 1 {
		t.Fatalf("recv=%v ack=%v", dst.recvSubmitted, src.ackSubmitted)
	}
	if got.effectTxHash() != got.AckTxHash {
		t.Fatalf("effect = %q, want ack %q", got.effectTxHash(), got.AckTxHash)
	}
}

// A recv that mines as a Noop (an earlier recv won the race) carries no ack in
// its receipt; the relayer must fall back to scanning WriteAcknowledgement.
func TestRecvNoopFallsBackToScannedAck(t *testing.T) {
	st := openTestStore(t)
	p := insertPendingPacket(t, st, 1, 0)
	src, dst := newFakeOps(), newFakeOps()
	dst.recvNoopSeqs[1] = true
	r := testRelayer(st, src, dst, true)

	if err := r.reconcilePacket(t.Context(), p, nil); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	got := packetByID(t, st, p.PacketID)
	if got.State != relayercmd.PacketComplete {
		t.Fatalf("state = %q, want complete", got.State)
	}
	if got.AckHex != testSuccessAck {
		t.Fatalf("ack_hex = %q, want %q", got.AckHex, testSuccessAck)
	}
	if len(dst.recvSubmitted) != 1 || len(src.ackSubmitted) != 1 {
		t.Fatalf("recv=%v ack=%v", dst.recvSubmitted, src.ackSubmitted)
	}
}

func TestErrorAckClassification(t *testing.T) {
	st := openTestStore(t)
	p := insertPendingPacket(t, st, 2, 0)
	src, dst := newFakeOps(), newFakeOps()
	dst.recvAckBySeq[2] = append([]byte(nil), errorAckSentinel...)
	r := testRelayer(st, src, dst, true)

	if err := r.reconcilePacket(t.Context(), p, nil); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	got := packetByID(t, st, p.PacketID)
	if got.State != relayercmd.PacketErrorAck {
		t.Fatalf("state = %q, want error_ack", got.State)
	}
	if got.Reason != errorAckReason {
		t.Fatalf("reason = %q, want %q", got.Reason, errorAckReason)
	}
}

func TestTimeoutWhenNotReceived(t *testing.T) {
	st := openTestStore(t)
	p := insertPendingPacket(t, st, 3, 100)
	src, dst := newFakeOps(), newFakeOps()
	dst.time = 100
	r := testRelayer(st, src, dst, true)

	if err := r.reconcilePacket(t.Context(), p, nil); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	got := packetByID(t, st, p.PacketID)
	if got.State != relayercmd.PacketTimedOut {
		t.Fatalf("state = %q, want timed_out", got.State)
	}
	if len(src.timeoutSubmitted) != 1 {
		t.Fatalf("timeout submitted = %v", src.timeoutSubmitted)
	}
	if len(dst.recvSubmitted) != 0 {
		t.Fatalf("recv should not run after timeout: %v", dst.recvSubmitted)
	}
	if got.Reason == "" || got.AckTxHash == "" {
		t.Fatalf("timed_out packet missing reason/tx: %+v", got)
	}
}

func TestNoTimeoutWhenAlreadyReceived(t *testing.T) {
	st := openTestStore(t)
	p := insertPendingPacket(t, st, 4, 100)
	src, dst := newFakeOps(), newFakeOps()
	dst.time = 200
	dst.writeAcks[ackKey{client: testDstClient, seq: 4}] = ackCapture{
		Ack: []byte{0x01}, TxHash: common.HexToHash(testRecvTx),
	}
	r := testRelayer(st, src, dst, true)

	if err := r.reconcilePacket(t.Context(), p, nil); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	got := packetByID(t, st, p.PacketID)
	if got.State != relayercmd.PacketComplete {
		t.Fatalf("state = %q, want complete", got.State)
	}
	if len(src.timeoutSubmitted) != 0 {
		t.Fatalf("timeout should not be submitted: %v", src.timeoutSubmitted)
	}
	if len(dst.recvSubmitted) != 0 {
		t.Fatalf("recv should use scan: %v", dst.recvSubmitted)
	}
}

func TestResumeFromWriteAckScan(t *testing.T) {
	st := openTestStore(t)
	p := insertPendingPacket(t, st, 5, 0)
	src, dst := newFakeOps(), newFakeOps()
	dst.writeAcks[ackKey{client: testDstClient, seq: 5}] = ackCapture{
		Ack: []byte{0x99}, TxHash: common.HexToHash("0xalready"),
	}
	r := testRelayer(st, src, dst, true)

	if err := r.reconcilePacket(t.Context(), p, nil); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	got := packetByID(t, st, p.PacketID)
	if got.State != relayercmd.PacketComplete {
		t.Fatalf("state = %q, want complete", got.State)
	}
	if got.RecvTxHash != common.HexToHash("0xalready").Hex() {
		t.Fatalf("recv tx = %q", got.RecvTxHash)
	}
	if len(dst.recvSubmitted) != 0 {
		t.Fatalf("recv submitted = %v; want scan-only resume", dst.recvSubmitted)
	}
	if len(src.ackSubmitted) != 1 {
		t.Fatalf("ack submitted = %v; want 1", src.ackSubmitted)
	}
}

func TestResumeAdoptSourceAck(t *testing.T) {
	st := openTestStore(t)
	p := insertPendingPacket(t, st, 6, 0)
	src, dst := newFakeOps(), newFakeOps()
	src.ackPackets[ackKey{client: testSrcClient, seq: 6}] = ackCapture{
		Ack: []byte{0x01}, TxHash: common.HexToHash("0xacked"),
	}
	r := testRelayer(st, src, dst, true)

	if err := r.reconcilePacket(t.Context(), p, nil); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	got := packetByID(t, st, p.PacketID)
	if got.State != relayercmd.PacketComplete || got.AckTxHash != common.HexToHash("0xacked").Hex() {
		t.Fatalf("got = %+v", got)
	}
	if len(dst.recvSubmitted) != 0 || len(src.ackSubmitted) != 0 {
		t.Fatalf("should adopt terminal without submits: recv=%v ack=%v", dst.recvSubmitted, src.ackSubmitted)
	}
}

func TestManualRouteGating(t *testing.T) {
	st := openTestStore(t)
	p := insertPendingPacket(t, st, 7, 0)
	src, dst := newFakeOps(), newFakeOps()
	dst.recvAckBySeq[7] = []byte{0x01}
	r := testRelayer(st, src, dst, false)

	if err := r.reconcilePacket(t.Context(), p, map[relayRequestKey]bool{}); err != nil {
		t.Fatalf("reconcile without request: %v", err)
	}
	got := packetByID(t, st, p.PacketID)
	if got.State != relayercmd.PacketPending {
		t.Fatalf("state = %q; want still pending", got.State)
	}
	if len(dst.recvSubmitted) != 0 {
		t.Fatalf("relayed without request: %v", dst.recvSubmitted)
	}

	requested := map[relayRequestKey]bool{{SourceChainID: testSrcChain, SourceTxHash: testSourceTx}: true}
	if err := r.reconcilePacket(t.Context(), p, requested); err != nil {
		t.Fatalf("reconcile with request: %v", err)
	}
	got = packetByID(t, st, p.PacketID)
	if got.State != relayercmd.PacketComplete {
		t.Fatalf("state = %q, want complete after request", got.State)
	}
}

func TestCrashResumeWithStoredAckHex(t *testing.T) {
	st := openTestStore(t)
	p := insertPendingPacket(t, st, 8, 0)
	if err := st.MarkReceived(t.Context(), p.PacketID, testRecvTx, testSuccessAck); err != nil {
		t.Fatal(err)
	}
	p = packetByID(t, st, p.PacketID)
	src, dst := newFakeOps(), newFakeOps()
	r := testRelayer(st, src, dst, true)

	if err := r.reconcilePacket(t.Context(), p, nil); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	got := packetByID(t, st, p.PacketID)
	if got.State != relayercmd.PacketComplete {
		t.Fatalf("state = %q", got.State)
	}
	if len(dst.recvSubmitted) != 0 {
		t.Fatalf("recv should be skipped when ack_hex stored: %v", dst.recvSubmitted)
	}
	if len(src.ackSubmitted) != 1 {
		t.Fatalf("ack submitted = %v", src.ackSubmitted)
	}
}

func TestIsErrorAck(t *testing.T) {
	if !isErrorAck(errorAckSentinel) {
		t.Fatal("sentinel should classify as error ack")
	}
	if isErrorAck([]byte{0x01}) {
		t.Fatal("success ack misclassified")
	}
	decoded, err := hexutil.Decode("0x4774d4a575993f963b1c06573736617a457abef8589178db8d10c94b4ab511ab")
	if err != nil {
		t.Fatal(err)
	}
	if !isErrorAck(decoded) {
		t.Fatal("hex-decoded sentinel should match")
	}
}
