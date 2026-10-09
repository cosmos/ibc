// SPDX-License-Identifier: Apache-2.0

package relayerload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/e2e/internal/loadtest/stats"
)

// fakeRelayer lists statuses that advance one state per listing.
type fakeRelayer struct {
	mu       sync.Mutex
	listings int
	// progress maps sequence to the states it reports on successive listings;
	// the last state sticks.
	progress map[uint64][]relayerv2.PacketState
	failNext bool
}

func (f *fakeRelayer) list(context.Context) ([]*relayerv2.PacketStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext {
		f.failNext = false
		return nil, errors.New("unavailable")
	}
	listing := f.listings
	f.listings++
	var statuses []*relayerv2.PacketStatus
	for sequence, states := range f.progress {
		state := states[min(listing, len(states)-1)]
		statuses = append(statuses, &relayerv2.PacketStatus{
			SourceClientId: "client-0", SequenceNumber: sequence, State: state,
			RecvTx: &relayerv2.TransactionInfo{TxHash: "0x01"},
			AckTx:  &relayerv2.TransactionInfo{TxHash: "0x02"},
		})
	}
	// A packet of another client is ignored.
	statuses = append(statuses, &relayerv2.PacketStatus{SourceClientId: "client-9", SequenceNumber: 1})
	return statuses, nil
}

func sendAll(packets ...sentPacket) <-chan sentPacket {
	sent := make(chan sentPacket, len(packets))
	for _, p := range packets {
		sent <- p
	}
	close(sent)
	return sent
}

const (
	pending   = relayerv2.PacketState_PACKET_STATE_PENDING
	succeeded = relayerv2.PacketState_PACKET_STATE_SUCCEEDED
	timedOut  = relayerv2.PacketState_PACKET_STATE_TIMED_OUT
)

func TestSweepEndsWhenEveryPacketIsTerminal(t *testing.T) {
	relayer := &fakeRelayer{failNext: true, progress: map[uint64][]relayerv2.PacketState{
		1: {pending, succeeded},
		2: {pending, pending, timedOut},
	}}
	sent := sendAll(
		sentPacket{key: packetKey{"client-0", 1}, block: 7},
		sentPacket{key: packetKey{"client-0", 2}, block: 8},
	)

	packets, err := sweep(t.Context(), "test", time.Millisecond, time.Minute, relayer.list, sent)

	require.NoError(t, err)
	require.Len(t, packets, 2)
	assert.Equal(t, succeeded, packets[0].state())
	assert.Equal(t, uint64(7), packets[0].sendBlock)
	assert.Equal(t, timedOut, packets[1].state())
	assert.Equal(t, 3, relayer.listings, "a failed listing is retried; the third shows both terminal")
}

func TestSweepWaitsForSendingToFinish(t *testing.T) {
	relayer := &fakeRelayer{progress: map[uint64][]relayerv2.PacketState{1: {succeeded}, 2: {succeeded}}}
	sent := make(chan sentPacket, 2)
	sent <- sentPacket{key: packetKey{"client-0", 1}}
	done := make(chan []*trackedPacket)
	go func() {
		packets, _ := sweep(t.Context(), "test", time.Millisecond, time.Minute, relayer.list, sent)
		done <- packets
	}()
	time.Sleep(20 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("sweep ended while sending was still open")
	default:
	}
	sent <- sentPacket{key: packetKey{"client-0", 2}}
	close(sent)
	assert.Len(t, <-done, 2)
}

func TestSweepGivesUpWhenRelayingStalls(t *testing.T) {
	relayer := &fakeRelayer{progress: map[uint64][]relayerv2.PacketState{1: {pending, succeeded}, 2: {pending}}}
	sent := sendAll(
		sentPacket{key: packetKey{"client-0", 1}},
		sentPacket{key: packetKey{"client-0", 2}},
		sentPacket{key: packetKey{"client-0", 3}},
	)

	packets, err := sweep(t.Context(), "test", time.Millisecond, 20*time.Millisecond, relayer.list, sent)

	require.ErrorContains(t, err, "2 of 3 packets not terminal: no relay progress for 20ms")
	assert.Equal(t, succeeded, packets[0].state(), "progress before the stall is kept")
	assert.False(t, packets[1].terminal())
	assert.Nil(t, packets[2].status, "never listed")
}

func TestSweepGivesUpWhenListingKeepsFailing(t *testing.T) {
	list := func(context.Context) ([]*relayerv2.PacketStatus, error) { return nil, errors.New("unavailable") }

	_, err := sweep(t.Context(), "test", time.Millisecond, 20*time.Millisecond, list,
		sendAll(sentPacket{key: packetKey{"client-0", 1}}))

	require.ErrorContains(t, err, "1 of 1 packets not terminal: no relay progress for 20ms")
	require.ErrorContains(t, err, "last listing error: unavailable")
}

func TestSweepExplainsAnInterruption(t *testing.T) {
	relayer := &fakeRelayer{progress: map[uint64][]relayerv2.PacketState{1: {pending}}}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()

	_, err := sweep(ctx, "test", time.Millisecond, time.Minute, relayer.list,
		sendAll(sentPacket{key: packetKey{"client-0", 1}}))

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorContains(t, err, "1 of 1 packets not terminal")
}

// fakeChain serves receipts and headers from maps.
type fakeChain struct {
	blocks map[common.Hash]uint64
	times  map[uint64]uint64
	mu     sync.Mutex
	reads  int
}

func (c *fakeChain) TransactionReceipt(_ context.Context, hash common.Hash) (*types.Receipt, error) {
	c.mu.Lock()
	c.reads++
	c.mu.Unlock()
	number, ok := c.blocks[hash]
	if !ok {
		return nil, errors.New("not found")
	}
	return &types.Receipt{BlockNumber: new(big.Int).SetUint64(number)}, nil
}

func (c *fakeChain) HeaderByNumber(_ context.Context, number *big.Int) (*types.Header, error) {
	c.mu.Lock()
	c.reads++
	c.mu.Unlock()
	return &types.Header{Number: number, Time: c.times[number.Uint64()]}, nil
}

func TestRelayLatencies(t *testing.T) {
	status := func(state relayerv2.PacketState, recv, ack string) *relayerv2.PacketStatus {
		return &relayerv2.PacketStatus{
			State:  state,
			RecvTx: &relayerv2.TransactionInfo{TxHash: recv},
			AckTx:  &relayerv2.TransactionInfo{TxHash: ack},
		}
	}
	source := &fakeChain{
		blocks: map[common.Hash]uint64{common.HexToHash("0xa1"): 20},
		times:  map[uint64]uint64{10: 1000, 11: 1001, 20: 1010},
	}
	destination := &fakeChain{
		blocks: map[common.Hash]uint64{common.HexToHash("0xb1"): 5},
		times:  map[uint64]uint64{5: 1004},
	}
	packets := []*trackedPacket{
		// Two packets share one receive and one acknowledgement transaction.
		{sendBlock: 10, status: status(succeeded, "0xb1", "0xa1")},
		{sendBlock: 11, status: status(succeeded, "0xb1", "0xa1")},
		// Not succeeded: excluded, and its unknown transactions are never read.
		{sendBlock: 11, status: status(timedOut, "0xdead", "0xbeef")},
		{sendBlock: 11},
	}

	sendToRecv, recvToAck, sendToAck, err := relayLatencies(t.Context(), source, destination, packets)

	require.NoError(t, err)
	assert.Equal(t, []time.Duration{4 * time.Second, 3 * time.Second}, sendToRecv)
	assert.Equal(t, []time.Duration{6 * time.Second, 6 * time.Second}, recvToAck)
	assert.Equal(t, []time.Duration{10 * time.Second, 9 * time.Second}, sendToAck)
	assert.Equal(t, 1+3, source.reads, "one receipt and three headers, each read once")
	assert.Equal(t, 1+1, destination.reads)
}

func TestRelayLatenciesSkipsPacketsWithFailedReads(t *testing.T) {
	status := func(recv string) *relayerv2.PacketStatus {
		return &relayerv2.PacketStatus{
			State:  succeeded,
			RecvTx: &relayerv2.TransactionInfo{TxHash: recv},
			AckTx:  &relayerv2.TransactionInfo{TxHash: "0xa1"},
		}
	}
	source := &fakeChain{
		blocks: map[common.Hash]uint64{common.HexToHash("0xa1"): 20},
		times:  map[uint64]uint64{10: 1000, 20: 1010},
	}
	destination := &fakeChain{
		blocks: map[common.Hash]uint64{common.HexToHash("0xb1"): 5},
		times:  map[uint64]uint64{5: 1004},
	}
	packets := []*trackedPacket{
		{sendBlock: 10, status: status("0xb1")},
		// Its receive receipt is missing.
		{sendBlock: 10, status: status("0xdead")},
	}

	sendToRecv, _, sendToAck, err := relayLatencies(t.Context(), source, destination, packets)

	require.ErrorContains(t, err, "not found")
	assert.Equal(t, []time.Duration{4 * time.Second}, sendToRecv, "the other packet is still measured")
	assert.Equal(t, []time.Duration{10 * time.Second}, sendToAck)
}

func TestVerdictSeparatesTheRelayerFromTheLoad(t *testing.T) {
	short := func() *Report {
		return &Report{Directions: []*DirectionReport{
			{Route: "a-to-b", Target: 5, Sent: 5},
			{Route: "b-to-a", Target: 5, Sent: 4, SendFailures: 1, FirstSendError: "boom"},
		}}
	}

	report := short()
	err := verdict(report, nil)
	require.ErrorIs(t, err, ErrLoadNotDelivered)
	require.ErrorContains(t, err, "b-to-a: sent 4 of 5 packets (1 send failures, first: boom)")
	assert.True(t, report.Passed, "the relayer relayed every packet sent")
	assert.False(t, report.LoadDelivered)

	report = short()
	err = verdict(report, errors.New("b-to-a: 1 packets did not succeed"))
	require.NotErrorIs(t, err, ErrLoadNotDelivered, "a relayer failure is the verdict")
	assert.False(t, report.Passed)

	report = &Report{Directions: []*DirectionReport{{Target: 5, Sent: 5}}}
	require.NoError(t, verdict(report, nil))
	assert.True(t, report.Passed)
	assert.True(t, report.LoadDelivered)
}

func TestReportJSONAndText(t *testing.T) {
	report := &Report{
		Scenario: "relayer", Provider: "anvil", Wallets: 4, PerBlock: 2, Packets: 10,
		WallTimeMS: 5000, Throughput: 2, Error: "b-to-a: sent 4 of 5 packets",
		Directions: []*DirectionReport{{
			Route: "a-to-b", Source: "chain-a", Destination: "chain-b", Target: 5, Sent: 5, Succeeded: 5,
			SendToRecv: stats.Summarize([]time.Duration{time.Second}),
			relayDone:  time.Now(),
		}},
	}

	encoded, err := json.Marshal(report)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.Equal(t, false, decoded["passed"])
	assert.InDelta(t, 2, decoded["throughput_packets_per_second"], 0)
	direction := decoded["directions"].([]any)[0].(map[string]any)
	assert.InDelta(t, 5, direction["succeeded"], 0)
	assert.InDelta(t, 1000, direction["send_to_recv"].(map[string]any)["p50_ms"], 0)
	assert.NotContains(t, direction, "relayDone")

	var out bytes.Buffer
	require.NoError(t, report.WriteText(&out))
	text := out.String()
	assert.Contains(t, text,
		"Relayer load: FAIL (anvil, 4 wallets, 2 per block interval): 10 packets in 5s, 2.0 packets/s")
	assert.Contains(t, text, "[a-to-b] chain-a -> chain-b")
	assert.Regexp(t, `send->recv +\(block time\): +n=1 p50=1s`, text)
	assert.Contains(t, text, "b-to-a: sent 4 of 5 packets")
}
