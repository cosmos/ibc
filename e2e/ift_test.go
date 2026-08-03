package e2e_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"

	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

// zeroAddressReceiver is a well-formed but invalid IFT receiver: the
// destination mint reverts on it, forcing an error acknowledgement.
var zeroAddressReceiver = (common.Address{}).Hex()

const (
	attestedClientAID  environment.ClientID    = "attested-client-a"
	attestedClientBID  environment.ClientID    = "attested-client-b"
	attestorAID        environment.AttestorID  = "attestor-a"
	attestorBID        environment.AttestorID  = "attestor-b"
	attestorAAuthority environment.AuthorityID = "attestor-a-authority"
	attestorBAuthority environment.AuthorityID = "attestor-b-authority"
)

func TestAttestedIFTTransfer_AutoRelay(t *testing.T) {
	t.Parallel()
	spec := environment.Spec{
		Chains: e2etest.ChainSpecsForConfiguredLane(t),
		IBCInstances: []environment.IBCInstanceSpec{
			environment.NewIBCInstance{
				ID:        "attested-ibc-a",
				Chain:     e2etest.ChainA,
				Authority: e2etest.ProtocolAuthorityID,
			},
			environment.NewIBCInstance{
				ID:        "attested-ibc-b",
				Chain:     e2etest.ChainB,
				Authority: e2etest.ProtocolAuthorityID,
			},
		},
		Connections: []environment.ConnectionSpec{{
			ID: "attested-connection",
			A: environment.NewClient{
				ID:                    attestedClientAID,
				IBCInstance:           "attested-ibc-a",
				Authority:             e2etest.ProtocolAuthorityID,
				MinRequiredSignatures: 1,
			},
			B: environment.NewClient{
				ID:                    attestedClientBID,
				IBCInstance:           "attested-ibc-b",
				Authority:             e2etest.ProtocolAuthorityID,
				MinRequiredSignatures: 1,
			},
		}},
		Attestors: []environment.AttestorSpec{
			{ID: attestorAID, Client: attestedClientAID, Authority: attestorAAuthority},
			{ID: attestorBID, Client: attestedClientBID, Authority: attestorBAuthority},
		},
	}
	runtime := e2etest.RuntimeWithProtocolDeployer(
		environment.Runtime{Authorities: map[environment.AuthorityID]environment.EVMAuthority{
			attestorAAuthority: {PrivateKeyHex: "0000000000000000000000000000000000000000000000000000000000000006"},
			attestorBAuthority: {PrivateKeyHex: "0000000000000000000000000000000000000000000000000000000000000007"},
		}},
	)
	env := e2etest.Start(t, spec, runtime)
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	attestorA, err := env.Attestor(attestorAID)
	require.NoError(t, err)
	attestorB, err := env.Attestor(attestorBID)
	require.NoError(t, err)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	iftApp := e2etest.BindIFT(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: big.NewInt(1_234_000)})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyBurned(ctx))

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	status, err := e2etest.AwaitState(ctx, relayer, transfer.Packet(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED, destination.Timing())
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))

	source, err := env.Chain(route.Source)
	require.NoError(t, err)
	sourceEVM, err := source.EVM()
	require.NoError(t, err)
	sendReceipt, err := sourceEVM.TransactionReceipt(ctx, common.HexToHash(transfer.Packet().SourceTxHash))
	require.NoError(t, err)
	destinationEVM, err := destination.EVM()
	require.NoError(t, err)
	receiveReceipt, err := destinationEVM.TransactionReceipt(ctx, common.HexToHash(status.GetRecvTx().GetTxHash()))
	require.NoError(t, err)

	destinationState := attestedClientState(t, destinationEVM, attestorB.IBCClient().LightClientAddress())
	sourceState := attestedClientState(t, sourceEVM, attestorA.IBCClient().LightClientAddress())
	// The receive proof advanced the destination client through the send block.
	require.GreaterOrEqual(t, destinationState.LatestHeight, sendReceipt.BlockNumber.Uint64())
	// The acknowledgement proof advanced the source client through the receive block.
	require.GreaterOrEqual(t, sourceState.LatestHeight, receiveReceipt.BlockNumber.Uint64())
}

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
	require.NoError(t, transfer.VerifyPending(ctx))

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	_, err = e2etest.AwaitState(ctx, relayer, transfer.Packet(),
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
	_, err = e2etest.AwaitState(ctx, relayer, transfer.Packet(),
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
	_, err = e2etest.AwaitState(ctx, relayer, transfer.Packet(),
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
	_, err = e2etest.AwaitState(ctx, relayer, transfer.Packet(),
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
		_, err := e2etest.AwaitState(ctx, relayer, transfer.Packet(),
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
		_, err = e2etest.AwaitState(ctx, relayer, packet,
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
