// SPDX-License-Identifier: Apache-2.0

package pipeline

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"pgregory.net/rapid"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/relay/processors"
	"github.com/cosmos/ibc/cli/internal/relay/prover"
	"github.com/cosmos/ibc/cli/internal/store"
	"github.com/cosmos/ibc/cli/internal/txsubmitter"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// TestPacketLifecycleModel drives the real pipeline stages against a modeled
// pair of chains while rapid interleaves sends, relay passes and faults:
// mined, reverted, dropped and expired txs, lagging light clients, a
// congested tx submitter, and another relayer racing ours.
//
// After every step the store must agree with the chains: no packet records
// both an ack and a timeout, a terminal status names the tx that settled the
// packet on chain, and it never changes. The relayer must never build a relay
// tx the chain state rules out, and no transfer may leave a pass with an
// ErrInvariant, or with no error and no terminal status. Once the faults
// stop, every packet must settle within a bounded number of passes.
//
// The stages run one transfer and one batch at a time, so a failing sequence
// replays exactly and rapid can shrink it.
func TestPacketLifecycleModel(t *testing.T) {
	rapid.Check(t, packetLifecycleProperty)
}

// FuzzPacketLifecycleModel runs the same model under the daily fuzz budget.
func FuzzPacketLifecycleModel(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(packetLifecycleProperty))
}

func packetLifecycleProperty(t *rapid.T) {
	m := newLifecycleModel(t, lifecycleConfig{
		updatePayloads:   rapid.IntRange(1, 2).Draw(t, "updatePayloads"),
		recvBatchSize:    rapid.IntRange(1, 4).Draw(t, "recvBatchSize"),
		ackBatchSize:     rapid.IntRange(1, 4).Draw(t, "ackBatchSize"),
		timeoutBatchSize: rapid.IntRange(1, 4).Draw(t, "timeoutBatchSize"),
	})
	defer m.close()

	t.Repeat(rapid.StateMachineActions(m))
	m.quiesce(t)
}

// TestPacketReceivedAndAckedByAnotherRelayer replays the races the model
// found: another relayer receives and acks a packet while our recv for it is
// pending, and ours then lands as a no-op or reverts. The packet must still
// settle with the other relayer's txs.
func TestPacketReceivedAndAckedByAnotherRelayer(t *testing.T) {
	for _, revert := range []bool{false, true} {
		t.Run(fmt.Sprintf("ourRecvReverts=%t", revert), func(t *testing.T) {
			m := newLifecycleModel(t, lifecycleConfig{
				updatePayloads: 1, recvBatchSize: 1, ackBatchSize: 1, timeoutBatchSize: 1,
			})
			defer m.close()

			m.send(t, false, v2.WriteAckStatusSuccess)
			m.UpdateDestinationClient(t)
			m.Relay(t)

			ours := m.w.pending()
			if len(ours) != 1 || ours[0].items[0].Kind != v2.RelayKindRecv {
				t.Fatalf("expected our recv tx pending, got %d pending txs", len(ours))
			}

			p := m.w.packets[0]
			m.w.otherRelay(p, v2.RelayKindRecv)
			m.w.otherRelay(p, v2.RelayKindAck)
			m.w.mine(ours[0], revert)
			m.check(t)

			m.quiesce(t)
		})
	}
}

const (
	modelMaxPackets = 6
	// modelQuiescePasses bounds how many fault-free rounds of mining,
	// light-client updates and relay passes every packet may take to settle.
	modelQuiescePasses = 10

	ourRelayer   = "0x00000000000000000000000000000000000000aa"
	otherRelayer = "0x00000000000000000000000000000000000000bb"
)

type lifecycleConfig struct {
	updatePayloads   int
	recvBatchSize    int
	ackBatchSize     int
	timeoutBatchSize int
}

type mTxState int

const (
	txPending mTxState = iota
	txMined
	txReverted
)

// mTx a transaction on one of the modeled chains: a send, or a relay of items.
type mTx struct {
	hash    string
	chainID string
	sent    *mPacket
	items   []v2.PacketRelayItem
	relayer string
	state   mTxState
	// expired the tx submitter has given up waiting for it
	expired bool
	// dropped it left the mempool and will never be mined
	dropped bool
	height  uint64
	minedAt time.Time
	// executed sequences whose router operation ran when the tx was mined;
	// an operation that was no longer valid is a no-op
	executed map[uint64]bool
}

// mPacket one packet's state across both chains.
type mPacket struct {
	packet  channeltypesv2.Packet
	timeout time.Time
	sendTx  *mTx
	// ackStatus the write ack the destination application produces on recv
	ackStatus v2.WriteAckStatus

	committed bool // commitment live on the source chain
	recvTx    *mTx // tx that executed the recv on the destination
	ackTx     *mTx // tx that executed the ack on the source
	timeoutTx *mTx // tx that executed the timeout on the source
}

func (p *mPacket) received() bool { return p.recvTx != nil }

func hashOf(tx *mTx) string {
	if tx == nil {
		return ""
	}
	return tx.hash
}

// lifecycleWorld the two chains, their light clients and mempools. The stages
// run sequentially, but CheckPacketCommitment races two lookups, so access is
// locked.
type lifecycleWorld struct {
	mu sync.Mutex

	srcHeight, dstHeight uint64
	// dstTime the destination chain's block time. It never passes the wall
	// clock: the relayer's IsTimedOut reads the wall clock, and a destination
	// ahead of it would refuse recvs the relayer keeps sending.
	dstTime time.Time

	// srcProvable the source height the destination's client can prove
	srcProvable uint64
	// dstProvable/dstProvableTime the destination height and block time the
	// source's client can prove
	dstProvable     uint64
	dstProvableTime time.Time

	packets []*mPacket // index sequence-1
	txs     map[string]*mTx
	txOrder []*mTx
	built   [][]v2.PacketRelayItem
	hashes  int

	updatePayloads int
	// busy the tx submitters refuse new submissions (txsubmitter.ErrBusy)
	busy bool

	violations []string
}

func (w *lifecycleWorld) violate(format string, args ...any) {
	w.violations = append(w.violations, fmt.Sprintf(format, args...))
}

func (w *lifecycleWorld) packet(sequence uint64) *mPacket {
	if sequence == 0 || sequence > uint64(len(w.packets)) {
		return nil
	}
	return w.packets[sequence-1]
}

func (w *lifecycleWorld) addTx(chainID, relayer string) *mTx {
	w.hashes++
	tx := &mTx{hash: fmt.Sprintf("0x%064x", w.hashes), chainID: chainID, relayer: relayer}
	w.txs[tx.hash] = tx
	w.txOrder = append(w.txOrder, tx)

	return tx
}

func (w *lifecycleWorld) pending() []*mTx {
	var pending []*mTx
	for _, tx := range w.txOrder {
		if tx.state == txPending {
			pending = append(pending, tx)
		}
	}
	return pending
}

// valid reports whether item would execute if mined now on chainID.
func (w *lifecycleWorld) valid(chainID string, item v2.PacketRelayItem) bool {
	p := w.packet(item.Packet.Sequence)
	if p == nil {
		return false
	}

	switch item.Kind {
	case v2.RelayKindRecv:
		return chainID == testRoute.DestinationChainID && !p.received() && w.dstTime.Before(p.timeout)
	case v2.RelayKindAck:
		return chainID == testRoute.SourceChainID && p.committed && p.received()
	case v2.RelayKindTimeout:
		return chainID == testRoute.SourceChainID && p.committed && !p.received() && !w.dstTime.Before(p.timeout)
	default:
		return false
	}
}

// mine includes tx in the next block of its chain, executing each of its
// operations that is still valid.
func (w *lifecycleWorld) mine(tx *mTx, revert bool) {
	if tx.chainID == testRoute.SourceChainID {
		w.srcHeight++
		tx.height = w.srcHeight
	} else {
		w.dstHeight++
		tx.height = w.dstHeight
	}

	tx.minedAt = time.Now().UTC()

	if revert {
		tx.state = txReverted
		return
	}

	tx.state = txMined
	tx.executed = make(map[uint64]bool)

	for _, item := range tx.items {
		if !w.valid(tx.chainID, item) {
			continue
		}

		p := w.packet(item.Packet.Sequence)
		tx.executed[p.packet.Sequence] = true

		switch item.Kind {
		case v2.RelayKindRecv:
			p.recvTx = tx
		case v2.RelayKindAck:
			p.committed, p.ackTx = false, tx
		case v2.RelayKindTimeout:
			p.committed, p.timeoutTx = false, tx
		}
	}
}

// otherRelay has another relayer's tx performing kind for p mined at once.
func (w *lifecycleWorld) otherRelay(p *mPacket, kind v2.RelayKind) {
	chainID := testRoute.SourceChainID
	if kind == v2.RelayKindRecv {
		chainID = testRoute.DestinationChainID
	}

	tx := w.addTx(chainID, otherRelayer)
	tx.items = []v2.PacketRelayItem{{Kind: kind, Packet: p.packet}}
	w.mine(tx, false)
}

// modelChain serves one chain's reads from the world, as seen from the
// route's client on that chain. Methods the pipeline is not expected to call
// panic through the nil embedded interface.
type modelChain struct {
	chains.Client

	w        *lifecycleWorld
	chainID  string
	clientID string
}

func (c *modelChain) ChainID() string { return c.chainID }

func (c *modelChain) WaitForChain(context.Context) error { return nil }

// expect records a violation unless the call is asked of chainID about the
// client the route has there.
func (c *modelChain) expect(call, chainID, clientID string) bool {
	if c.chainID != chainID || c.clientID != clientID {
		c.w.violate("%s asked of chain %s about client %q", call, c.chainID, clientID)
		return false
	}
	return true
}

// tx returns a tx on this chain that has been mined, successfully or not.
func (c *modelChain) tx(hash string) (*mTx, bool) {
	tx, ok := c.w.txs[hash]
	if !ok || tx.chainID != c.chainID || tx.state == txPending {
		return nil, false
	}
	return tx, true
}

func (c *modelChain) TxPacketEvents(_ context.Context, txHash []byte) ([]v2.PacketEvent, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()

	tx, ok := c.tx("0x" + hex.EncodeToString(txHash))
	if !ok || tx.state != txMined {
		return nil, v2.ErrTxNotFound
	}

	if tx.sent != nil {
		return []v2.PacketEvent{{
			Height: tx.height, BlockTime: tx.minedAt, Kind: v2.KindSendPacket, Packet: tx.sent.packet, TxHash: tx.hash,
		}}, nil
	}

	var events []v2.PacketEvent
	for _, item := range tx.items {
		if item.Kind != v2.RelayKindRecv || !tx.executed[item.Packet.Sequence] {
			continue
		}
		events = append(events, v2.PacketEvent{
			Height:    tx.height,
			BlockTime: tx.minedAt,
			Kind:      v2.KindWriteAck,
			Packet:    item.Packet,
			Acks:      [][]byte{{byte(c.w.packet(item.Packet.Sequence).ackStatus)}},
			TxHash:    tx.hash,
		})
	}

	return events, nil
}

func (c *modelChain) TxHeight(_ context.Context, txHash []byte) (uint64, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()

	tx, ok := c.tx("0x" + hex.EncodeToString(txHash))
	if !ok {
		return 0, v2.ErrTxNotFound
	}

	return tx.height, nil
}

func (c *modelChain) IsPacketReceived(_ context.Context, clientID string, sequence uint64) (bool, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()

	if !c.expect("IsPacketReceived", testRoute.DestinationChainID, clientID) {
		return false, errors.New("wrong chain or client")
	}

	return c.w.packet(sequence).received(), nil
}

func (c *modelChain) IsPacketCommitted(_ context.Context, clientID string, sequence uint64) (bool, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()

	if !c.expect("IsPacketCommitted", testRoute.SourceChainID, clientID) {
		return false, errors.New("wrong chain or client")
	}

	return c.w.packet(sequence).committed, nil
}

func (c *modelChain) findTx(call, chainID, clientID string, sequence uint64, pick func(*mPacket) *mTx) (*v2.Tx, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()

	if !c.expect(call, chainID, clientID) {
		return nil, errors.New("wrong chain or client")
	}

	tx := pick(c.w.packet(sequence))
	if tx == nil {
		return nil, v2.ErrTxNotFound
	}

	return &v2.Tx{Hash: tx.hash, Timestamp: tx.minedAt, RelayerAddress: tx.relayer}, nil
}

func (c *modelChain) FindRecvTx(_ context.Context, clientID string, sequence uint64) (*v2.Tx, error) {
	return c.findTx("FindRecvTx", testRoute.DestinationChainID, clientID, sequence,
		func(p *mPacket) *mTx { return p.recvTx })
}

func (c *modelChain) FindAckTx(_ context.Context, clientID string, sequence uint64) (*v2.Tx, error) {
	return c.findTx("FindAckTx", testRoute.SourceChainID, clientID, sequence,
		func(p *mPacket) *mTx { return p.ackTx })
}

func (c *modelChain) FindTimeoutTx(_ context.Context, clientID string, sequence uint64) (*v2.Tx, error) {
	return c.findTx("FindTimeoutTx", testRoute.SourceChainID, clientID, sequence,
		func(p *mPacket) *mTx { return p.timeoutTx })
}

// expectClients records a violation unless a packet-identifying call names
// the route's client pair.
func (c *modelChain) expectClients(call, sourceClientID, destClientID string) bool {
	if sourceClientID != testRoute.SourceClientID || destClientID != testRoute.DestinationClientID {
		c.w.violate("%s asked about clients %q -> %q", call, sourceClientID, destClientID)
		return false
	}
	return true
}

func (c *modelChain) PacketRelayExecuted(
	_ context.Context,
	txHash string,
	kind v2.RelayKind,
	sequence uint64,
	sourceClientID, destClientID string,
) (bool, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()

	if !c.expect("PacketRelayExecuted", testRoute.SourceChainID, testRoute.SourceClientID) ||
		!c.expectClients("PacketRelayExecuted", sourceClientID, destClientID) {
		return false, errors.New("wrong chain or client")
	}

	tx, ok := c.tx(txHash)
	if !ok || tx.state != txMined {
		return false, v2.ErrTxNotFound
	}

	for _, item := range tx.items {
		if item.Kind == kind && item.Packet.Sequence == sequence {
			return tx.executed[sequence], nil
		}
	}

	return false, nil
}

func (c *modelChain) PacketWriteAckStatus(
	_ context.Context,
	recvTxHash string,
	sequence uint64,
	sourceClientID, destClientID string,
) (v2.WriteAckStatus, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()

	if !c.expect("PacketWriteAckStatus", testRoute.DestinationChainID, testRoute.DestinationClientID) ||
		!c.expectClients("PacketWriteAckStatus", sourceClientID, destClientID) {
		return v2.WriteAckStatusUnknown, errors.New("wrong chain or client")
	}

	tx, ok := c.tx(recvTxHash)
	switch {
	case !ok:
		return v2.WriteAckStatusUnknown, errors.Errorf("no receipt for recv tx %s", recvTxHash)
	case !tx.executed[sequence]:
		return v2.WriteAckStatusUnknown, v2.ErrWriteAckNotFoundForPacket
	}

	return c.w.packet(sequence).ackStatus, nil
}

// modelProver a light client on one chain tracking the other. Like the Besu
// prover, it proves whatever it is asked to: keeping invalid claims out of a
// relay tx is the pipeline's job. onSource is the source chain's client of
// the destination.
type modelProver struct {
	w        *lifecycleWorld
	onSource bool
}

var _ prover.Prover = (*modelProver)(nil)

func (p *modelProver) LatestProvableHeight(context.Context) (uint64, time.Time, error) {
	p.w.mu.Lock()
	defer p.w.mu.Unlock()

	if p.onSource {
		return p.w.dstProvable, p.w.dstProvableTime, nil
	}

	return p.w.srcProvable, time.Time{}, nil
}

func (p *modelProver) ClientUpdatePayloads(context.Context, uint64) ([][]byte, error) {
	p.w.mu.Lock()
	defer p.w.mu.Unlock()

	payloads := make([][]byte, p.w.updatePayloads)
	for i := range payloads {
		payloads[i] = []byte{byte(i)}
	}

	return payloads, nil
}

func (p *modelProver) PacketProofs(
	_ context.Context,
	height uint64,
	kind v2.ProofKind,
	packets []channeltypesv2.Packet,
	_ []channeltypesv2.Acknowledgement,
) ([][]byte, error) {
	p.w.mu.Lock()
	defer p.w.mu.Unlock()

	provable := p.w.srcProvable
	if p.onSource {
		provable = p.w.dstProvable
	}

	if height > provable {
		p.w.violate("proof requested at height %d above provable height %d", height, provable)
	}

	// a claim the light client cannot yet prove fails the whole batch,
	// holding back every other packet in it
	for _, packet := range packets {
		mp := p.w.packet(packet.Sequence)
		switch {
		case kind == v2.ProofKindPacketCommitment && mp.sendTx.height > height:
			p.w.violate("commitment proof for packet %d sent at %d requested at height %d",
				packet.Sequence, mp.sendTx.height, height)
		case kind == v2.ProofKindAcknowledgement && mp.received() && mp.recvTx.height > height:
			p.w.violate("ack proof for packet %d written at %d requested at height %d",
				packet.Sequence, mp.recvTx.height, height)
		}
	}

	proofs := make([][]byte, len(packets))
	for i := range proofs {
		proofs[i] = []byte{byte(kind)}
	}

	return proofs, nil
}

// modelTxBuilder registers each built tx's items with the world and encodes
// their index as calldata, so the tx submitter knows what a tx does. It is
// the first point the relayer commits to an item, so it checks each one
// against the chains.
type modelTxBuilder struct {
	w *lifecycleWorld
}

var modelRouter = common.HexToAddress("0x00000000000000000000000000000000000000cc")

func (b *modelTxBuilder) BuildRelayTxs(_ v2.ClientUpdate, items []v2.PacketRelayItem) ([]v2.RelayTx, error) {
	b.w.mu.Lock()
	defer b.w.mu.Unlock()

	for _, item := range items {
		p := b.w.packet(item.Packet.Sequence)
		switch {
		case item.Kind == v2.RelayKindRecv && (!p.committed || p.received() || !b.w.dstTime.Before(p.timeout)):
			b.w.violate("recv built for packet %d: committed=%t received=%t destination past timeout=%t",
				p.packet.Sequence, p.committed, p.received(), !b.w.dstTime.Before(p.timeout))
		case item.Kind == v2.RelayKindAck && (!p.committed || !p.received()):
			b.w.violate("ack built for packet %d: committed=%t received=%t",
				p.packet.Sequence, p.committed, p.received())
		case item.Kind == v2.RelayKindTimeout && (!p.committed || p.received() || b.w.dstProvableTime.Before(p.timeout)):
			b.w.violate("timeout built for packet %d: committed=%t received in %q timeout provable=%t",
				p.packet.Sequence, p.committed, hashOf(p.recvTx), !b.w.dstProvableTime.Before(p.timeout))
		}
	}

	b.w.built = append(b.w.built, slices.Clone(items))

	return []v2.RelayTx{{
		To:   modelRouter.Bytes(),
		Data: binary.BigEndian.AppendUint64(nil, uint64(len(b.w.built)-1)),
	}}, nil
}

func (w *lifecycleWorld) itemsOf(data []byte) []v2.PacketRelayItem {
	return w.built[binary.BigEndian.Uint64(data)]
}

// modelTxSubmitter our relayer's signer on one chain.
type modelTxSubmitter struct {
	w       *lifecycleWorld
	chainID string
}

func (s *modelTxSubmitter) SimulateRelayTx(_ context.Context, tx v2.RelayTx) (v2.RelaySimulation, error) {
	s.w.mu.Lock()
	defer s.w.mu.Unlock()

	ops := []v2.OperationResult{{Type: v2.OperationTypeUpdate, Success: true}}
	for _, item := range s.w.itemsOf(tx.Data) {
		op := v2.OperationResult{Type: v2.OperationTypePacket, Success: s.w.valid(s.chainID, item)}
		if !op.Success {
			op.FailureReason = "invalid"
		}
		ops = append(ops, op)
	}

	return v2.RelaySimulation{Operations: ops}, nil
}

func (s *modelTxSubmitter) Submit(_ context.Context, intent v2.TxIntent) (*v2.Submission, error) {
	s.w.mu.Lock()
	defer s.w.mu.Unlock()

	items := s.w.itemsOf(intent.Data)

	if s.w.busy && len(items) > 0 {
		return nil, txsubmitter.ErrBusy
	}

	tx := s.w.addTx(s.chainID, ourRelayer)
	tx.items = items

	// update-only txs are mined at once: waitForInclusion would otherwise
	// poll on a wall-clock ticker
	if len(items) == 0 {
		s.w.mine(tx, false)
	}

	return &v2.Submission{TxHash: tx.hash, SubmittedAt: time.Now().UTC(), RelayerAddress: ourRelayer}, nil
}

func (s *modelTxSubmitter) ShouldRetry(_ context.Context, txHash string, _ time.Time) (bool, error) {
	s.w.mu.Lock()
	defer s.w.mu.Unlock()

	tx, ok := s.w.txs[txHash]
	switch {
	case !ok || tx.chainID != s.chainID:
		s.w.violate("ShouldRetry on chain %s for unknown tx %s", s.chainID, txHash)
		return false, errors.Errorf("unknown tx %s", txHash)
	case tx.state == txPending && tx.expired:
		return true, nil
	case tx.state == txPending:
		return false, v2.ErrTxNotFound
	default:
		return tx.state == txReverted, nil
	}
}

// lifecycleModel the rapid state machine. Actions with nothing to act on are
// no-ops rather than skips: fuzz input can pick only those, and rapid fails a
// run with no valid action.
type lifecycleModel struct {
	ctx   context.Context
	w     *lifecycleWorld
	db    *store.SqliteDB
	steps []stage

	// settled the terminal status each packet reached, which must not change
	settled map[uint64]store.RelayStatus
}

func newLifecycleModel(t rapid.TB, cfg lifecycleConfig) *lifecycleModel {
	db, err := store.NewSqliteInMemory()
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}

	if _, err = db.MigrateUp(); err != nil {
		_ = db.Close()
		t.Fatalf("migrating store: %v", err)
	}

	w := &lifecycleWorld{
		txs:            make(map[string]*mTx),
		updatePayloads: cfg.updatePayloads,
		// before every timeout the model draws, which lie an hour either side
		// of now
		dstTime: time.Now().Add(-2 * time.Hour).UTC(),
	}

	end := func(chainID, clientID string, onSource bool) processors.RouteEnd {
		return processors.RouteEnd{
			Chain:       &modelChain{w: w, chainID: chainID, clientID: clientID},
			Prover:      &modelProver{w: w, onSource: onSource},
			TxBuilder:   &modelTxBuilder{w: w},
			TxSubmitter: &modelTxSubmitter{w: w, chainID: chainID},
		}
	}

	route := processors.RouteDeps{
		Route:       testRoute,
		Source:      end(testRoute.SourceChainID, testRoute.SourceClientID, true),
		Destination: end(testRoute.DestinationChainID, testRoute.DestinationClientID, false),
	}

	opts := Options{
		RecvBatchSize:    cfg.recvBatchSize,
		AckBatchSize:     cfg.ackBatchSize,
		TimeoutBatchSize: cfg.timeoutBatchSize,
	}

	return &lifecycleModel{
		ctx:     context.Background(),
		w:       w,
		db:      db,
		steps:   stages(slog.New(slog.DiscardHandler), db, route, opts),
		settled: make(map[uint64]store.RelayStatus),
	}
}

func (m *lifecycleModel) close() { _ = m.db.Close() }

// Send emits a packet on the source chain and records it for relay, as the
// subscription does.
func (m *lifecycleModel) Send(t *rapid.T) {
	if len(m.w.packets) >= modelMaxPackets {
		return
	}

	m.send(t, rapid.Bool().Draw(t, "timedOut"), rapid.SampledFrom([]v2.WriteAckStatus{
		v2.WriteAckStatusSuccess, v2.WriteAckStatusError,
	}).Draw(t, "ackStatus"))
}

func (m *lifecycleModel) send(t rapid.TB, timedOut bool, ackStatus v2.WriteAckStatus) {
	// an hour from now either way: IsTimedOut reads the wall clock
	timeout := time.Now().Add(time.Hour)
	if timedOut {
		timeout = time.Now().Add(-time.Hour)
	}
	timeout = timeout.UTC().Truncate(time.Second)

	m.w.mu.Lock()
	p := &mPacket{
		packet: channeltypesv2.Packet{
			Sequence:          uint64(len(m.w.packets) + 1),
			SourceClient:      testRoute.SourceClientID,
			DestinationClient: testRoute.DestinationClientID,
			TimeoutTimestamp:  uint64(timeout.Unix()), //nolint:gosec // positive
			Payloads: []channeltypesv2.Payload{{
				SourcePort: "transfer", DestinationPort: "transfer", Version: "ics20-1", Value: []byte{1},
			}},
		},
		timeout:   timeout,
		ackStatus: ackStatus,
		committed: true,
	}
	p.sendTx = m.w.addTx(testRoute.SourceChainID, otherRelayer)
	p.sendTx.sent = p
	m.w.mine(p.sendTx, false)
	m.w.packets = append(m.w.packets, p)
	m.w.mu.Unlock()

	err := m.db.UpsertPacket(m.ctx, store.UpsertPacket{
		Status:                    store.RelayStatusPending,
		SourceChainID:             testRoute.SourceChainID,
		DestinationChainID:        testRoute.DestinationChainID,
		SourceTxHash:              p.sendTx.hash,
		SourceTxTime:              p.sendTx.minedAt,
		PacketSequenceNumber:      p.packet.Sequence,
		PacketSourceClientID:      testRoute.SourceClientID,
		PacketDestinationClientID: testRoute.DestinationClientID,
		PacketTimeoutTimestamp:    timeout,
	})
	if err != nil {
		t.Fatalf("recording packet: %v", err)
	}
}

// Relay runs one dispatcher pass: every dispatchable packet through every
// stage. No transfer may leave it with an invariant violation, or with no
// error and no terminal status.
func (m *lifecycleModel) Relay(t rapid.TB) {
	packets, err := m.db.ListDispatchablePackets(m.ctx)
	if err != nil {
		t.Fatalf("listing dispatchable packets: %v", err)
	}

	transfers := make([]*processors.Transfer, len(packets))
	for i, packet := range packets {
		transfers[i] = processors.NewTransfer(packet, slog.New(slog.DiscardHandler))
	}

	for _, step := range m.steps {
		transfers = m.runStage(t, step, transfers)
	}

	if len(transfers) != len(packets) {
		t.Fatalf("%d transfers entered the pipeline but %d left it", len(packets), len(transfers))
	}

	for _, tr := range transfers {
		switch {
		case errors.Is(tr.ProcessingError, processors.ErrInvariant):
			t.Fatalf("packet %d: %v", tr.PacketSequenceNumber, tr.ProcessingError)
		case tr.ProcessingError == nil && !store.IsTerminal(tr.Status):
			t.Fatalf(
				"packet %d left the pipeline in %s, neither errored nor settled",
				tr.PacketSequenceNumber,
				tr.Status,
			)
		}
	}
}

// runStage feeds a stage the way NewPipeline does: a batch stage collects
// only the transfers it accepts, everything else flows past.
func (m *lifecycleModel) runStage(t rapid.TB, step stage, in []*processors.Transfer) []*processors.Transfer {
	out := make([]*processors.Transfer, 0, len(in))

	if step.single != nil {
		for _, tr := range in {
			result, err := step.single.Process(m.ctx, tr)
			if err != nil || result == nil {
				t.Fatalf("stage dropped transfer %d: %v", tr.PacketSequenceNumber, err)
			}
			out = append(out, result)
		}

		return out
	}

	var batchable []*processors.Transfer

	for _, tr := range in {
		if !step.batch.Accepts(tr) {
			out = append(out, tr)
			continue
		}
		batchable = append(batchable, tr)
	}

	for batch := range slices.Chunk(batchable, step.batchSize) {
		result, err := step.batch.Process(m.ctx, batch)
		if err != nil {
			t.Fatalf("batch stage failed: %v", err)
		}
		out = append(out, result...)
	}

	return out
}

// Mine resolves each pending tx: mined, reverted, or left pending. A tx the
// submitter gave up on can still be mined.
func (m *lifecycleModel) Mine(t *rapid.T) {
	m.w.mu.Lock()
	defer m.w.mu.Unlock()

	for _, tx := range m.w.pending() {
		if tx.dropped {
			continue
		}

		switch rapid.IntRange(0, 2).Draw(t, "outcome "+tx.hash) {
		case 1:
			m.w.mine(tx, false)
		case 2:
			m.w.mine(tx, true)
		}
	}
}

func (m *lifecycleModel) drawPending(t *rapid.T, label string) *mTx {
	pending := m.w.pending()
	if len(pending) == 0 {
		return nil
	}
	return rapid.SampledFrom(pending).Draw(t, label)
}

// Drop evicts a pending tx from the mempool for good.
func (m *lifecycleModel) Drop(t *rapid.T) {
	m.w.mu.Lock()
	defer m.w.mu.Unlock()

	if tx := m.drawPending(t, "dropped"); tx != nil {
		tx.dropped = true
	}
}

// Expire makes the tx submitter give up on a pending tx, as it does once the
// tx has been pending past its retry interval.
func (m *lifecycleModel) Expire(t *rapid.T) {
	m.w.mu.Lock()
	defer m.w.mu.Unlock()

	if tx := m.drawPending(t, "expired"); tx != nil {
		tx.expired = true
	}
}

// UpdateDestinationClient brings the destination's client of the source chain
// up to the source head.
func (m *lifecycleModel) UpdateDestinationClient(rapid.TB) {
	m.w.mu.Lock()
	defer m.w.mu.Unlock()

	m.w.srcProvable = m.w.srcHeight
}

// UpdateSourceClient brings the source's client of the destination chain up to
// the destination head.
func (m *lifecycleModel) UpdateSourceClient(rapid.TB) {
	m.w.mu.Lock()
	defer m.w.mu.Unlock()

	m.w.dstProvable = m.w.dstHeight
	m.w.dstProvableTime = m.w.dstTime
}

// CatchUpDestinationTime moves the destination chain's block time up to the
// wall clock, past the timeout of every packet the relayer considers timed
// out.
func (m *lifecycleModel) CatchUpDestinationTime(rapid.TB) {
	m.w.mu.Lock()
	defer m.w.mu.Unlock()

	m.w.dstTime = time.Now().UTC()
}

// Congest toggles whether the tx submitters have capacity for new txs.
func (m *lifecycleModel) Congest(rapid.TB) {
	m.w.mu.Lock()
	defer m.w.mu.Unlock()

	m.w.busy = !m.w.busy
}

// otherRelayOne has another relayer perform kind for one of the packets it
// is valid for.
func (m *lifecycleModel) otherRelayOne(t *rapid.T, kind v2.RelayKind) {
	m.w.mu.Lock()
	defer m.w.mu.Unlock()

	chainID := testRoute.SourceChainID
	if kind == v2.RelayKindRecv {
		chainID = testRoute.DestinationChainID
	}

	var candidates []*mPacket
	for _, p := range m.w.packets {
		if m.w.valid(chainID, v2.PacketRelayItem{Kind: kind, Packet: p.packet}) {
			candidates = append(candidates, p)
		}
	}

	if len(candidates) > 0 {
		m.w.otherRelay(rapid.SampledFrom(candidates).Draw(t, "packet"), kind)
	}
}

// OtherRecv another relayer delivers a packet, which may be one the relayer
// already considers timed out while the destination does not yet.
func (m *lifecycleModel) OtherRecv(t *rapid.T) { m.otherRelayOne(t, v2.RelayKindRecv) }

// OtherAck another relayer acks a received packet.
func (m *lifecycleModel) OtherAck(t *rapid.T) { m.otherRelayOne(t, v2.RelayKindAck) }

// OtherTimeout another relayer times out a packet.
func (m *lifecycleModel) OtherTimeout(t *rapid.T) { m.otherRelayOne(t, v2.RelayKindTimeout) }

func (m *lifecycleModel) rows(t rapid.TB) []store.Packet {
	rows, err := m.db.ListPackets(
		m.ctx,
		store.PacketFilter{Statuses: store.AllRelayStatuses()},
		store.Page{Limit: 1000},
	)
	if err != nil {
		t.Fatalf("listing packets: %v", err)
	}
	return rows
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

// Check holds after every action.
func (m *lifecycleModel) Check(t *rapid.T) { m.check(t) }

func (m *lifecycleModel) check(t rapid.TB) {
	m.w.mu.Lock()
	defer m.w.mu.Unlock()

	if len(m.w.violations) > 0 {
		t.Fatalf("pipeline misused the chains:\n%s", strings.Join(m.w.violations, "\n"))
	}

	for _, row := range m.rows(t) {
		seq := row.PacketSequenceNumber
		p := m.w.packet(seq)

		if row.AckTxHash != nil && row.TimeoutTxHash != nil {
			t.Fatalf("packet %d records both ack tx %s and timeout tx %s", seq, *row.AckTxHash, *row.TimeoutTxHash)
		}

		if prev, ok := m.settled[seq]; ok && row.Status != prev {
			t.Fatalf("packet %d left terminal status %s for %s", seq, prev, row.Status)
		}

		switch row.Status {
		case store.RelayStatusCompleteWithTimeout:
			if deref(row.TimeoutTxHash) != hashOf(p.timeoutTx) || p.timeoutTx == nil {
				t.Fatalf("packet %d is %s with timeout tx %q, but the source chain timed it out in %q",
					seq, row.Status, deref(row.TimeoutTxHash), hashOf(p.timeoutTx))
			}
		case store.RelayStatusCompleteWithAck, store.RelayStatusCompleteWithWriteAckError:
			if deref(row.AckTxHash) != hashOf(p.ackTx) || p.ackTx == nil {
				t.Fatalf("packet %d is %s with ack tx %q, but the source chain acked it in %q",
					seq, row.Status, deref(row.AckTxHash), hashOf(p.ackTx))
			}
			if deref(row.RecvTxHash) != hashOf(p.recvTx) {
				t.Fatalf("packet %d is %s with recv tx %q, but the destination received it in %q",
					seq, row.Status, deref(row.RecvTxHash), hashOf(p.recvTx))
			}
			if (row.Status == store.RelayStatusCompleteWithWriteAckError) != (p.ackStatus == v2.WriteAckStatusError) {
				t.Fatalf("packet %d is %s but its write ack status is %d", seq, row.Status, p.ackStatus)
			}
		case store.RelayStatusFailed:
			t.Fatalf("packet %d marked %s", seq, row.Status)
		}

		if store.IsTerminal(row.Status) {
			m.settled[seq] = row.Status
		}
	}
}

// quiesce stops the faults: whatever is stuck in a mempool expires, everything
// else is mined, the destination's time catches up and the light clients keep
// up. Every packet must then settle.
func (m *lifecycleModel) quiesce(t rapid.TB) {
	for range modelQuiescePasses {
		m.w.mu.Lock()
		for _, tx := range m.w.pending() {
			if tx.dropped {
				tx.expired = true
				continue
			}
			m.w.mine(tx, false)
		}
		m.w.busy = false
		m.w.mu.Unlock()

		m.CatchUpDestinationTime(t)
		m.UpdateDestinationClient(t)
		m.UpdateSourceClient(t)

		m.Relay(t)
		m.check(t)

		dispatchable, err := m.db.ListDispatchablePackets(m.ctx)
		if err != nil {
			t.Fatalf("listing dispatchable packets: %v", err)
		}
		if len(dispatchable) == 0 {
			return
		}
	}

	var stuck []string
	for _, row := range m.rows(t) {
		if !store.IsTerminal(row.Status) {
			stuck = append(stuck, fmt.Sprintf("packet %d in %s (recv %q, ack %q, timeout %q)",
				row.PacketSequenceNumber, row.Status,
				deref(row.RecvTxHash), deref(row.AckTxHash), deref(row.TimeoutTxHash)))
		}
	}

	t.Fatalf("packets did not settle within %d fault-free passes:\n%s",
		modelQuiescePasses, strings.Join(stuck, "\n"))
}
