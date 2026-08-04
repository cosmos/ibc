package e2e_test

import (
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"

	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

// zeroAddressReceiver is a well-formed but invalid IFT receiver: the
// destination mint reverts on it, forcing an error acknowledgement.
var zeroAddressReceiver = (common.Address{}).Hex()

func TestIFTTransfer_AutoRelay(t *testing.T) {
	t.Parallel()
	spec := dummyClientMeshSpec(e2etest.ChainSpecsForConfiguredLane(t))
	runtime := e2etest.RuntimeWithProtocolDeployer(environment.Runtime{})
	env := e2etest.Start(t, spec, runtime)
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	iftApp := e2etest.BindIFT(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: big.NewInt(1_234_000)})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyBurned(ctx))
	require.NoError(t, transfer.VerifyPending(ctx))

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	err = e2etest.AwaitState(ctx, relayer, transfer.Packet(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED, destination.Timing())
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyPendingCleared(ctx))
	require.NoError(t, transfer.VerifyBurned(ctx), "a successful ack must not also refund")
}

func TestIFTTimeout_Refund(t *testing.T) {
	t.Parallel()
	e2etest.RequireAnvilLane(t)
	spec := dummyClientMeshSpec(e2etest.ChainSpecsForConfiguredLane(t))
	runtime := e2etest.RuntimeWithProtocolDeployer(environment.Runtime{})
	env := e2etest.Start(t, spec, runtime)
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	iftApp := e2etest.BindIFT(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	require.NoError(t, relayer.Stop(ctx))
	transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{
		Amount:  big.NewInt(3_000_000),
		Timeout: transferTimeout,
	})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyBurned(ctx))
	require.NoError(t, transfer.VerifyPending(ctx))

	chainB, err := env.Chain(route.Destination)
	require.NoError(t, err)
	mining, err := chainB.Mining()
	require.NoError(t, err)
	require.NoError(t, mining.AdvanceTime(ctx, transferTimeoutAdvance))
	relayer = e2etest.StartRelayer(t, driver, env)

	source, err := env.Chain(route.Source)
	require.NoError(t, err)
	err = e2etest.AwaitState(ctx, relayer, transfer.Packet(),
		relayerv2.PacketState_PACKET_STATE_TIMED_OUT, source.Timing())
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyRefunded(ctx))
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	require.NoError(t, transfer.VerifyPendingCleared(ctx))
}

// TestIFTTransfer_ErrorAck_Refund sends to the zero address, which the
// destination mint rejects, forcing an error acknowledgement and a refund.
func TestIFTTransfer_ErrorAck_Refund(t *testing.T) {
	t.Parallel()
	spec := dummyClientMeshSpec(e2etest.ChainSpecsForConfiguredLane(t))
	runtime := e2etest.RuntimeWithProtocolDeployer(environment.Runtime{})
	env := e2etest.Start(t, spec, runtime)
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	iftApp := e2etest.BindIFT(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{
		Amount:   big.NewInt(1_234_000),
		Receiver: zeroAddressReceiver,
	})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyBurned(ctx))
	require.NoError(t, transfer.VerifyPending(ctx))

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	err = e2etest.AwaitState(ctx, relayer, transfer.Packet(),
		relayerv2.PacketState_PACKET_STATE_REJECTED, destination.Timing())
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	require.NoError(t, transfer.VerifyRefunded(ctx))
	require.NoError(t, transfer.VerifyPendingCleared(ctx))
}

// TestIFTTransfer_ErrorAck_UnregisteredBridge leaves the destination IFT
// bridge unregistered, so onRecvPacket hits IFTBridgeNotFound.
func TestIFTTransfer_ErrorAck_UnregisteredBridge(t *testing.T) {
	t.Parallel()
	spec := dummyClientMeshSpec(e2etest.ChainSpecsForConfiguredLane(t))
	runtime := e2etest.RuntimeWithProtocolDeployer(environment.Runtime{})
	env := e2etest.Start(t, spec, runtime)
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	route.SkipDestinationIFTBridge = true
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	iftApp := e2etest.BindIFT(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: big.NewInt(1_234_000)})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyBurned(ctx))
	require.NoError(t, transfer.VerifyPending(ctx))

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	err = e2etest.AwaitState(ctx, relayer, transfer.Packet(),
		relayerv2.PacketState_PACKET_STATE_REJECTED, destination.Timing())
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	require.NoError(t, transfer.VerifyRefunded(ctx))
	require.NoError(t, transfer.VerifyPendingCleared(ctx))
}

// TestIFTTransfer_MultiPacketPending sends several transfers before any are
// relayed, so each sequence must get its own pending record and must only be
// cleared by its own acknowledgement.
func TestIFTTransfer_MultiPacketPending(t *testing.T) {
	t.Parallel()
	spec := dummyClientMeshSpec(e2etest.ChainSpecsForConfiguredLane(t))
	runtime := e2etest.RuntimeWithProtocolDeployer(environment.Runtime{})
	env := e2etest.Start(t, spec, runtime)
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	iftApp := e2etest.BindIFT(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	require.NoError(t, relayer.Stop(ctx))
	amounts := []*big.Int{big.NewInt(1_000_000), big.NewInt(2_000_000), big.NewInt(3_000_000)}
	transfers := make([]*e2etest.IFTPacket, len(amounts))
	for i, amount := range amounts {
		transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: amount})
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyBurned(ctx))
		require.NoError(t, transfer.VerifyPending(ctx))
		transfers[i] = transfer
	}

	relayer = e2etest.StartRelayer(t, driver, env)
	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	for _, transfer := range transfers {
		err := e2etest.AwaitState(ctx, relayer, transfer.Packet(),
			relayerv2.PacketState_PACKET_STATE_SUCCEEDED, destination.Timing())
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyDelivered(ctx))
		require.NoError(t, transfer.VerifyPendingCleared(ctx))
	}
	require.NoError(t, transfers[len(transfers)-1].VerifyBurned(ctx), "successful acks must not also refund")
}

// TestIFTTransfer_MultiPacketSingleTx emits multiple packets from a single
// source transaction via the batching shim, then relays them all with one
// Relay call, so the relayer must discover, track, and complete every packet
// from that one transaction — none missing, none duplicated.
func TestIFTTransfer_MultiPacketSingleTx(t *testing.T) {
	t.Parallel()
	spec := dummyClientMeshSpec(e2etest.ChainSpecsForConfiguredLane(t))
	runtime := e2etest.RuntimeWithProtocolDeployer(environment.Runtime{})
	env := e2etest.Start(t, spec, runtime)
	signers := e2etest.NewSigners(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	iftApp := e2etest.BindIFT(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	const packetCount = 10
	requests := make([]e2etest.IFTRequest, packetCount)
	for i := range requests {
		requests[i] = e2etest.IFTRequest{Amount: big.NewInt(int64(1_000_000 + i))}
	}
	batch, err := iftApp.SendBatch(ctx, requests)
	require.NoError(t, err)
	packets := batch.Packets()
	require.Len(t, packets, packetCount)

	wantSequences := make(map[uint64]struct{}, packetCount)
	for _, packet := range packets {
		wantSequences[packet.Sequence] = struct{}{}
	}
	require.Len(t, wantSequences, packetCount, "packets must have distinct sequences")

	require.NoError(t, e2etest.Relay(ctx, relayer, packets[0]))

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	for _, packet := range packets {
		err = e2etest.AwaitState(ctx, relayer, packet,
			relayerv2.PacketState_PACKET_STATE_SUCCEEDED, destination.Timing())
		require.NoError(t, err)
	}
	require.NoError(t, batch.VerifyDelivered(ctx))
	require.NoError(t, batch.VerifyBurned(ctx))

	statuses, err := relayer.PacketStatuses(ctx, string(route.Source), packets[0].SourceTxHash)
	require.NoError(t, err)
	require.Len(t, statuses, packetCount)
	gotSequences := make(map[uint64]struct{}, packetCount)
	for _, status := range statuses {
		require.Equal(t, relayerv2.PacketState_PACKET_STATE_SUCCEEDED, status.State)
		gotSequences[status.SequenceNumber] = struct{}{}
	}
	require.Equal(t, wantSequences, gotSequences)
}

const (
	batchTestPacketBatchSize    = 5
	batchTestPacketBatchTimeout = 5 * time.Second
)

// withBatchOverride raises PacketBatchSize/PacketBatchTimeout on every chain
// in the route above the harness's pinned defaults, scoped to this test's
// isolated environment.
func withBatchOverride(cfg *ibclink.RelayerConfig) {
	for i := range cfg.Chains {
		cfg.Chains[i].PacketBatchSize = batchTestPacketBatchSize
		cfg.Chains[i].PacketBatchTimeout = batchTestPacketBatchTimeout
	}
}

// TestIFTTransfer_BatchedRecvAck sends 10 IFT transfers as 10 separate source
// transactions, relays all 10 concurrently, and asserts the relayer batches
// their recv and ack transactions rather than submitting one of each per
// packet. The batching assertions are timing-robust: they check that fewer
// tx were used than packets and that at least one tx carried multiple
// packets, never that everything landed in a single tx, since that would
// race the dispatch poll interval and flake.
func TestIFTTransfer_BatchedRecvAck(t *testing.T) {
	t.Parallel()
	spec := dummyClientMeshSpec(e2etest.ChainSpecsForConfiguredLane(t))
	runtime := e2etest.RuntimeWithProtocolDeployer(environment.Runtime{})
	env := e2etest.Start(t, spec, runtime)
	signers := e2etest.NewSigners(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, signers, withBatchOverride, route)
	iftApp := e2etest.BindIFT(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	const packetCount = 10
	packets := make([]*e2etest.IFTPacket, packetCount)
	for i := range packets {
		transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: big.NewInt(int64(1_000_000 + i))})
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyBurned(ctx))
		packets[i] = transfer
	}

	relayErrs := make([]error, packetCount)
	var wg sync.WaitGroup
	for i, packet := range packets {
		wg.Add(1)
		go func(i int, packet *e2etest.IFTPacket) {
			defer wg.Done()
			relayErrs[i] = e2etest.Relay(ctx, relayer, packet.Packet())
		}(i, packet)
	}
	wg.Wait()
	for i, err := range relayErrs {
		require.NoErrorf(t, err, "relay call for packet %d (seq %d) failed", i, packets[i].Packet().Sequence)
	}

	for i, packet := range packets {
		statuses, err := relayer.PacketStatuses(ctx, string(route.Source), packet.Packet().SourceTxHash)
		require.NoError(t, err)
		require.Lenf(t, statuses, 1, "packet %d status should only include its own transaction", i)
		require.Equal(t, packet.Packet().Sequence, statuses[0].SequenceNumber)
	}

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	recvTxCounts := make(map[string]int, packetCount)
	ackTxCounts := make(map[string]int, packetCount)
	for _, packet := range packets {
		err = e2etest.AwaitState(ctx, relayer, packet.Packet(),
			relayerv2.PacketState_PACKET_STATE_SUCCEEDED, destination.Timing())
		require.NoError(t, err)
		require.NoError(t, packet.VerifyDelivered(ctx))

		statuses, err := relayer.PacketStatuses(ctx, string(route.Source), packet.Packet().SourceTxHash)
		require.NoError(t, err)
		require.Len(t, statuses, 1)
		recvTxCounts[statuses[0].GetRecvTx().GetTxHash()]++
		ackTxCounts[statuses[0].GetAckTx().GetTxHash()]++
	}

	require.Lessf(t, len(recvTxCounts), packetCount,
		"expected batched recv, got one recv tx per packet: %v", recvTxCounts)
	require.Lessf(t, len(ackTxCounts), packetCount,
		"expected batched ack, got one ack tx per packet: %v", ackTxCounts)
	require.Truef(t, hasBatchedTx(recvTxCounts), "no recv tx carried multiple packets: %v", recvTxCounts)
	require.Truef(t, hasBatchedTx(ackTxCounts), "no ack tx carried multiple packets: %v", ackTxCounts)
}

// hasBatchedTx reports whether any transaction hash in counts covers more
// than one packet.
func hasBatchedTx(counts map[string]int) bool {
	for _, count := range counts {
		if count >= 2 {
			return true
		}
	}
	return false
}

// TestIFTTransfer_BatchedRecvAckTimeout sends fewer IFT transfers than the
// configured PacketBatchSize, so the batch can only be released by
// PacketBatchTimeout elapsing rather than filling up. Both packets are
// submitted well within that timeout, so the relayer should still flush them
// together as a single recv tx and a single ack tx once the timeout fires.
func TestIFTTransfer_BatchedRecvAckTimeout(t *testing.T) {
	t.Parallel()
	spec := dummyClientMeshSpec(e2etest.ChainSpecsForConfiguredLane(t))
	runtime := e2etest.RuntimeWithProtocolDeployer(environment.Runtime{})
	env := e2etest.Start(t, spec, runtime)
	signers := e2etest.NewSigners(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, signers, withBatchOverride, route)
	iftApp := e2etest.BindIFT(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	const packetCount = batchTestPacketBatchSize - 1
	packets := make([]*e2etest.IFTPacket, packetCount)
	for i := range packets {
		transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: big.NewInt(int64(1_000_000 + i))})
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyBurned(ctx))
		packets[i] = transfer
	}

	for _, packet := range packets {
		require.NoError(t, e2etest.Relay(ctx, relayer, packet.Packet()))
	}

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	recvTxCounts := make(map[string]int, packetCount)
	ackTxCounts := make(map[string]int, packetCount)
	for _, packet := range packets {
		err = e2etest.AwaitState(ctx, relayer, packet.Packet(),
			relayerv2.PacketState_PACKET_STATE_SUCCEEDED, destination.Timing())
		require.NoError(t, err)
		require.NoError(t, packet.VerifyDelivered(ctx))

		statuses, err := relayer.PacketStatuses(ctx, string(route.Source), packet.Packet().SourceTxHash)
		require.NoError(t, err)
		require.Len(t, statuses, 1)
		recvTxCounts[statuses[0].GetRecvTx().GetTxHash()]++
		ackTxCounts[statuses[0].GetAckTx().GetTxHash()]++
	}

	require.Lenf(t, recvTxCounts, 1,
		"expected the batch timeout to flush both packets in one recv tx: %v", recvTxCounts)
	require.Lenf(t, ackTxCounts, 1,
		"expected the batch timeout to flush both packets in one ack tx: %v", ackTxCounts)
}
