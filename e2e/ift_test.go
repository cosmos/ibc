package e2e_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"

	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

//nolint:dupl // acceptance tests keep their setup sequences deliberately explicit
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

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	err = e2etest.AwaitState(ctx, relayer, transfer.Packet(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED, destination.Timing())
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
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
	require.Len(t, batch.Packets(), packetCount)

	wantSequences := make(map[uint64]struct{}, packetCount)
	for _, packet := range batch.Packets() {
		wantSequences[packet.Packet().Sequence] = struct{}{}
	}
	require.Len(t, wantSequences, packetCount, "packets must have distinct sequences")

	require.NoError(t, e2etest.Relay(ctx, relayer, batch.Packets()[0].Packet()))

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	for _, packet := range batch.Packets() {
		err = e2etest.AwaitState(ctx, relayer, packet.Packet(),
			relayerv2.PacketState_PACKET_STATE_SUCCEEDED, destination.Timing())
		require.NoError(t, err)
		require.NoError(t, packet.VerifyDelivered(ctx))
	}
	require.NoError(t, batch.VerifyBurned(ctx))

	statuses, err := relayer.PacketStatuses(ctx, string(route.Source), batch.Packets()[0].Packet().SourceTxHash)
	require.NoError(t, err)
	require.Len(t, statuses, packetCount)
	gotSequences := make(map[uint64]struct{}, packetCount)
	for _, status := range statuses {
		require.Equal(t, relayerv2.PacketState_PACKET_STATE_SUCCEEDED, status.State)
		gotSequences[status.SequenceNumber] = struct{}{}
	}
	require.Equal(t, wantSequences, gotSequences)
}
