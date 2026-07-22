package ibcrelay

import (
	"context"
	"math/big"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// fakeOps is an in-memory chainOps used by unit tests. Discovery
// (scanSendPackets/sendPacketsFromTx) is exercised end-to-end by the e2e
// acceptance tests and stays empty here.
type fakeOps struct {
	time uint64

	writeAcks    map[ackKey]ackCapture
	ackPackets   map[ackKey]ackCapture
	timeouts     map[ackKey]common.Hash
	recvAckBySeq map[uint64][]byte // ack returned by submitRecv
	recvNoopSeqs map[uint64]bool   // recv mines as a Noop: an earlier recv already wrote the ack

	recvSubmitted    []uint64
	ackSubmitted     []ackSubmit
	timeoutSubmitted []uint64

	nextTx uint64
}

type ackKey struct {
	client string
	seq    uint64
}

type ackSubmit struct {
	Seq uint64
	Ack []byte
}

func newFakeOps() *fakeOps {
	return &fakeOps{
		writeAcks:    map[ackKey]ackCapture{},
		ackPackets:   map[ackKey]ackCapture{},
		timeouts:     map[ackKey]common.Hash{},
		recvAckBySeq: map[uint64][]byte{},
		recvNoopSeqs: map[uint64]bool{},
		nextTx:       1,
	}
}

func (f *fakeOps) mintTx() common.Hash {
	f.nextTx++
	return common.BigToHash(new(big.Int).SetUint64(f.nextTx))
}

func (f *fakeOps) blockTimestamp(context.Context) (uint64, error) {
	return f.time, nil
}

func (f *fakeOps) submitRecv(_ context.Context, packet ics26router.IICS26RouterMsgsPacket) (*types.Receipt, error) {
	f.recvSubmitted = append(f.recvSubmitted, packet.Sequence)
	tx := f.mintTx()
	rcpt := &types.Receipt{TxHash: tx, Status: types.ReceiptStatusSuccessful}
	ack := f.recvAckBySeq[packet.Sequence]
	if ack == nil {
		ack = []byte{0x01}
	}
	if f.recvNoopSeqs[packet.Sequence] {
		// The recv mined as a Noop: an earlier recv landed first and wrote the
		// ack under its own transaction, so this receipt carries none.
		f.writeAcks[ackKey{client: packet.DestClient, seq: packet.Sequence}] = ackCapture{Ack: ack, TxHash: f.mintTx()}
		return rcpt, nil
	}
	f.writeAcks[ackKey{client: packet.DestClient, seq: packet.Sequence}] = ackCapture{Ack: ack, TxHash: tx}
	rcpt.Logs = []*types.Log{{TxHash: tx}} // marker so writeAckFromReceipt can find via side channel
	return rcpt, nil
}

func (f *fakeOps) writeAckFromReceipt(rcpt *types.Receipt) ([]byte, bool, error) {
	for _, capture := range f.writeAcks {
		if capture.TxHash == rcpt.TxHash {
			return capture.Ack, true, nil
		}
	}
	return nil, false, nil
}

func (f *fakeOps) submitAck(
	_ context.Context,
	packet ics26router.IICS26RouterMsgsPacket,
	ack []byte,
) (*types.Receipt, error) {
	f.ackSubmitted = append(f.ackSubmitted, ackSubmit{Seq: packet.Sequence, Ack: append([]byte(nil), ack...)})
	tx := f.mintTx()
	f.ackPackets[ackKey{client: packet.SourceClient, seq: packet.Sequence}] = ackCapture{
		Ack: append([]byte(nil), ack...), TxHash: tx,
	}
	return &types.Receipt{TxHash: tx, Status: types.ReceiptStatusSuccessful}, nil
}

func (f *fakeOps) submitTimeout(_ context.Context, packet ics26router.IICS26RouterMsgsPacket) (*types.Receipt, error) {
	f.timeoutSubmitted = append(f.timeoutSubmitted, packet.Sequence)
	tx := f.mintTx()
	f.timeouts[ackKey{client: packet.SourceClient, seq: packet.Sequence}] = tx
	return &types.Receipt{TxHash: tx, Status: types.ReceiptStatusSuccessful}, nil
}

func (f *fakeOps) findWriteAck(_ context.Context, destClient string, seq uint64) (ackCapture, bool, error) {
	capture, ok := f.writeAcks[ackKey{client: destClient, seq: seq}]
	return capture, ok, nil
}

func (f *fakeOps) findAckPacket(_ context.Context, sourceClient string, seq uint64) (ackCapture, bool, error) {
	capture, ok := f.ackPackets[ackKey{client: sourceClient, seq: seq}]
	return capture, ok, nil
}

func (f *fakeOps) findTimeoutPacket(_ context.Context, sourceClient string, seq uint64) (common.Hash, bool, error) {
	tx, ok := f.timeouts[ackKey{client: sourceClient, seq: seq}]
	return tx, ok, nil
}

func (f *fakeOps) scanSendPackets(context.Context, uint64) ([]sentPacket, uint64, error) {
	return nil, 1, nil
}

func (f *fakeOps) sendPacketsFromTx(context.Context, common.Hash) ([]sentPacket, error) {
	return nil, nil
}
