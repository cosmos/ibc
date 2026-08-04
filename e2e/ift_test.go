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

const (
	attestedClientAID  environment.ClientID    = "attested-client-a"
	attestedClientBID  environment.ClientID    = "attested-client-b"
	attestorAID        environment.AttestorID  = "attestor-a"
	attestorBID        environment.AttestorID  = "attestor-b"
	attestorCID        environment.AttestorID  = "attestor-c"
	attestorDID        environment.AttestorID  = "attestor-d"
	attestorAAuthority environment.AuthorityID = "attestor-a-authority"
	attestorBAuthority environment.AuthorityID = "attestor-b-authority"
	attestorCAuthority environment.AuthorityID = "attestor-c-authority"
	attestorDAuthority environment.AuthorityID = "attestor-d-authority"
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
	destinationEVM, err := destination.EVM()
	require.NoError(t, err)
	requireAttestedIFTClientHeights(t, sourceEVM, destinationEVM,
		attestorA.IBCClient().LightClientAddress(), attestorB.IBCClient().LightClientAddress(),
		transfer, status.GetRecvTx().GetTxHash())
}

func TestAttestedIFTTransfer_MultiAttestorQuorum(t *testing.T) {
	t.Parallel()
	spec := environment.Spec{
		Chains: e2etest.ChainSpecsForConfiguredLane(t),
		IBCInstances: []environment.IBCInstanceSpec{
			environment.NewIBCInstance{
				ID:        "attested-quorum-ibc-a",
				Chain:     e2etest.ChainA,
				Authority: e2etest.ProtocolAuthorityID,
			},
			environment.NewIBCInstance{
				ID:        "attested-quorum-ibc-b",
				Chain:     e2etest.ChainB,
				Authority: e2etest.ProtocolAuthorityID,
			},
		},
		Connections: []environment.ConnectionSpec{{
			ID: "attested-quorum-connection",
			A: environment.NewClient{
				ID: attestedClientAID, IBCInstance: "attested-quorum-ibc-a", Authority: e2etest.ProtocolAuthorityID,
				MinRequiredSignatures: 1,
			},
			B: environment.NewClient{
				ID: attestedClientBID, IBCInstance: "attested-quorum-ibc-b", Authority: e2etest.ProtocolAuthorityID,
				MinRequiredSignatures: 2,
			},
		}},
		Attestors: []environment.AttestorSpec{
			{ID: attestorAID, Client: attestedClientAID, Authority: attestorAAuthority},
			{ID: attestorBID, Client: attestedClientBID, Authority: attestorBAuthority},
			{ID: attestorCID, Client: attestedClientBID, Authority: attestorCAuthority},
			{ID: attestorDID, Client: attestedClientBID, Authority: attestorDAuthority},
		},
	}
	runtime := e2etest.RuntimeWithProtocolDeployer(
		environment.Runtime{Authorities: map[environment.AuthorityID]environment.EVMAuthority{
			attestorAAuthority: {
				PrivateKeyHex: "0000000000000000000000000000000000000000000000000000000000000006",
			},
			attestorBAuthority: {
				PrivateKeyHex: "0000000000000000000000000000000000000000000000000000000000000007",
			},
			attestorCAuthority: {
				PrivateKeyHex: "0000000000000000000000000000000000000000000000000000000000000008",
			},
			attestorDAuthority: {
				PrivateKeyHex: "0000000000000000000000000000000000000000000000000000000000000009",
			},
		}},
	)
	env := e2etest.Start(t, spec, runtime)
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	sourceAttestor, err := env.Attestor(attestorAID)
	require.NoError(t, err)
	destinationAttestorB, err := env.Attestor(attestorBID)
	require.NoError(t, err)
	destinationAttestorC, err := env.Attestor(attestorCID)
	require.NoError(t, err)
	destinationAttestorD, err := env.Attestor(attestorDID)
	require.NoError(t, err)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	iftApp := e2etest.BindIFT(t, env, deployment, signers, route)
	ctx := t.Context()
	// Keep every endpoint in Link's config while starting it with one attestor unavailable.
	require.NoError(t, destinationAttestorD.Stop(ctx))
	relayer := e2etest.StartRelayer(t, driver, env)

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	destinationEVM, err := destination.EVM()
	require.NoError(t, err)
	destinationClient := destinationAttestorB.IBCClient()
	require.ElementsMatch(t, []environment.EVMAddress{
		destinationAttestorB.SignerAddress(),
		destinationAttestorC.SignerAddress(),
		destinationAttestorD.SignerAddress(),
	}, destinationClient.AttestorAddresses())
	require.Equal(t, uint8(2), destinationClient.MinRequiredSignatures())

	// Two live attestors satisfy the destination client's quorum.
	transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: big.NewInt(1_234_000)})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyBurned(ctx))
	status, err := e2etest.AwaitState(ctx, relayer, transfer.Packet(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED, destination.Timing())
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))

	source, err := env.Chain(route.Source)
	require.NoError(t, err)
	sourceEVM, err := source.EVM()
	require.NoError(t, err)
	sendBlock := requireAttestedIFTClientHeights(t, sourceEVM, destinationEVM,
		sourceAttestor.IBCClient().LightClientAddress(), destinationClient.LightClientAddress(),
		transfer, status.GetRecvTx().GetTxHash())
	destinationAttestorBHeight, err := destinationAttestorB.LatestHeight(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, destinationAttestorBHeight, sendBlock)
	destinationAttestorCHeight, err := destinationAttestorC.LatestHeight(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, destinationAttestorCHeight, sendBlock)

	// One live attestor cannot satisfy the destination client's quorum.
	require.NoError(t, destinationAttestorC.Stop(ctx))
	pending, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: big.NewInt(2_345_000)})
	require.NoError(t, err)
	require.NoError(t, pending.VerifyBurned(ctx))
	require.NoError(t, e2etest.Relay(ctx, relayer, pending.Packet()))
	require.NoError(t, e2etest.AwaitStable(ctx, relayer, pending.Packet(),
		relayerv2.PacketState_PACKET_STATE_PENDING, destination.Timing()))
	require.NoError(t, pending.VerifyNotMinted(ctx))

	// Restoring a second attestor lets the pending transfer complete.
	require.NoError(t, destinationAttestorC.Restart(ctx))
	_, err = e2etest.AwaitState(ctx, relayer, pending.Packet(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED, destination.Timing())
	require.NoError(t, err)
	require.NoError(t, pending.VerifyDelivered(ctx))
}

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

const (
	batchTestPacketBatchSize    = 10
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
// packet.
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
		_, err = e2etest.AwaitState(ctx, relayer, packet.Packet(),
			relayerv2.PacketState_PACKET_STATE_SUCCEEDED, destination.Timing())
		require.NoError(t, err)
		require.NoError(t, packet.VerifyDelivered(ctx))

		statuses, err := relayer.PacketStatuses(ctx, string(route.Source), packet.Packet().SourceTxHash)
		require.NoError(t, err)
		require.Len(t, statuses, 1)
		recvTxCounts[statuses[0].GetRecvTx().GetTxHash()]++
		ackTxCounts[statuses[0].GetAckTx().GetTxHash()]++
	}
	require.NoError(t, packets[packetCount-1].VerifyBurned(ctx), "successful acks must not refund")

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

// TestIFTTransfer_BatchedTimeout sends more IFT transfers
// with a short packet timeout and asserts they are relayed
// in batches
func TestIFTTransfer_BatchedTimeout(t *testing.T) {
	t.Parallel()
	e2etest.RequireAnvilLane(t)
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
		transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{
			Amount:  big.NewInt(int64(1_000_000 + i)),
			Timeout: transferTimeout,
		})
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyBurned(ctx))
		packets[i] = transfer
	}

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	mining, err := destination.Mining()
	require.NoError(t, err)
	require.NoError(t, mining.AdvanceTime(ctx, transferTimeoutAdvance))

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

	source, err := env.Chain(route.Source)
	require.NoError(t, err)
	timeoutTxCounts := make(map[string]int, packetCount)
	for _, packet := range packets {
		_, err = e2etest.AwaitState(ctx, relayer, packet.Packet(),
			relayerv2.PacketState_PACKET_STATE_TIMED_OUT, source.Timing())
		require.NoError(t, err)
		require.NoError(t, packet.VerifyNotMinted(ctx))

		statuses, err := relayer.PacketStatuses(ctx, string(route.Source), packet.Packet().SourceTxHash)
		require.NoError(t, err)
		require.Len(t, statuses, 1)
		timeoutTxCounts[statuses[0].GetTimeoutTx().GetTxHash()]++
	}

	require.Lessf(t, len(timeoutTxCounts), packetCount,
		"expected batched timeouts, got one timeout tx per packet: %v", timeoutTxCounts)
	require.Truef(t, hasBatchedTx(timeoutTxCounts), "no timeout tx carried multiple packets: %v", timeoutTxCounts)
}
