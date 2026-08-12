// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

// zeroAddressReceiver is a well-formed but invalid IFT receiver: the
// destination mint reverts on it, forcing an error acknowledgement.
var zeroAddressReceiver = (common.Address{}).Hex()

func TestIFTTransfer_MultiAttestorQuorum(t *testing.T) {
	t.Parallel()
	const (
		attestorAID        environment.AttestorID  = "attestor-a"
		attestorBID        environment.AttestorID  = "attestor-b"
		attestorCID        environment.AttestorID  = "attestor-c"
		attestorDID        environment.AttestorID  = "attestor-d"
		attestorAAuthority environment.AuthorityID = "attestor-a-authority"
		attestorBAuthority environment.AuthorityID = "attestor-b-authority"
		attestorCAuthority environment.AuthorityID = "attestor-c-authority"
		attestorDAuthority environment.AuthorityID = "attestor-d-authority"
	)
	spec := environment.Spec{
		Chains: e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB),
		IBCInstances: []environment.IBCInstanceSpec{
			environment.NewIBCInstance{
				ID:        "quorum-ibc-a",
				Chain:     e2etest.ChainA,
				Authority: e2etest.ProtocolAuthorityID,
			},
			environment.NewIBCInstance{
				ID:        "quorum-ibc-b",
				Chain:     e2etest.ChainB,
				Authority: e2etest.ProtocolAuthorityID,
			},
		},
		Connections: []environment.ConnectionSpec{{
			ID: "quorum-connection",
			A: environment.NewClient{
				IBCInstance: "quorum-ibc-a", Authority: e2etest.ProtocolAuthorityID,
				MinRequiredSignatures: 1,
				Attestors: []environment.AttestorSpec{{
					ID: attestorAID, Authority: attestorAAuthority,
				}},
			},
			B: environment.NewClient{
				IBCInstance: "quorum-ibc-b", Authority: e2etest.ProtocolAuthorityID,
				MinRequiredSignatures: 2,
				Attestors: []environment.AttestorSpec{
					{ID: attestorBID, Authority: attestorBAuthority},
					{ID: attestorCID, Authority: attestorCAuthority},
					{ID: attestorDID, Authority: attestorDAuthority},
				},
			},
		}},
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
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	destinationAttestorB, err := env.Attestor(attestorBID)
	require.NoError(t, err)
	destinationAttestorC, err := env.Attestor(attestorCID)
	require.NoError(t, err)
	destinationAttestorD, err := env.Attestor(attestorDID)
	require.NoError(t, err)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	iftApp := e2etest.NewIFT(t, env, deployment, sender, route)
	ctx := t.Context()
	// Keep every endpoint in Link's config while starting it with one attestor unavailable.
	require.NoError(t, destinationAttestorD.Stop(ctx))
	relayer := e2etest.StartRelayer(t, driver, env)

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
	_, err = e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))

	// One live attestor cannot satisfy the destination client's quorum.
	require.NoError(t, destinationAttestorC.Stop(ctx))
	pending, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: big.NewInt(2_345_000)})
	require.NoError(t, err)
	require.NoError(t, pending.VerifyBurned(ctx))
	require.NoError(t, e2etest.RelayAll(ctx, relayer, pending.PacketTx()))
	require.NoError(t, e2etest.AwaitStable(ctx, relayer, pending.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_PENDING))
	require.NoError(t, pending.VerifyNotMinted(ctx))

	// Restoring a second attestor lets the pending transfer complete.
	require.NoError(t, destinationAttestorC.Restart(ctx))
	_, err = e2etest.AwaitState(ctx, relayer, pending.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, pending.VerifyDelivered(ctx))
}

func TestAttestedClient_MisbehaviourFreeze(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t,
		e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	destinationAttestorID := meshAttestorFor(route.Destination, route.Source)
	destinationAttestorKey := runtime.Authorities[environment.AuthorityID(destinationAttestorID)].PrivateKeyHex

	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	destinationAttestor, err := env.Attestor(destinationAttestorID)
	require.NoError(t, err)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	iftApp := e2etest.NewIFT(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	destinationEVM, err := destination.EVM()
	require.NoError(t, err)
	destinationClient := destinationAttestor.IBCClient()
	clientAddress := destinationClient.LightClientAddress()
	initialState := attestedClientState(t, destinationEVM, clientAddress)

	transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: big.NewInt(1_234_000)})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyBurned(ctx))
	_, err = e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))

	installedState := attestedClientState(t, destinationEVM, clientAddress)
	require.Greater(t, installedState.LatestHeight, initialState.LatestHeight)
	height := installedState.LatestHeight
	trustedTimestamp := attestedConsensusTimestamp(t, destinationEVM, clientAddress, height)
	proof := signedStateAttestationProof(t, destinationAttestorKey, height, trustedTimestamp+1)
	routerABI, err := ics26router.ContractMetaData.GetAbi()
	require.NoError(t, err)
	updateClient, err := routerABI.Pack("updateClient", destinationClient.ID(), proof)
	require.NoError(t, err)
	router := common.HexToAddress(string(destinationClient.IBCInstance().Locator()))
	require.NoError(t, sender.BroadcastTx(ctx, destinationEVM, router, updateClient))

	frozenState := attestedClientState(t, destinationEVM, clientAddress)
	require.True(t, frozenState.IsFrozen)
	require.Equal(t, height, frozenState.LatestHeight)
	require.Equal(t, trustedTimestamp, attestedConsensusTimestamp(t, destinationEVM, clientAddress, height))

	pending, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: big.NewInt(2_345_000)})
	require.NoError(t, err)
	require.NoError(t, pending.VerifyBurned(ctx))
	require.NoError(t, e2etest.RelayAll(ctx, relayer, pending.PacketTx()))
	require.NoError(t, e2etest.AwaitStable(ctx, relayer, pending.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_PENDING))
	status, err := e2etest.AwaitState(ctx, relayer, pending.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_PENDING)
	require.NoError(t, err)
	require.Empty(t, status.GetRecvTx().GetTxHash())
	require.NoError(t, pending.VerifyPending(ctx))
	require.NoError(t, pending.VerifyNotMinted(ctx))
}

// TestIFTTimeout_RefundUsesFinalizedDestinationAnchor proves timeouts anchor
// on the finalized destination header the attested client tracks — including
// when the source chain is far ahead of the destination.
func TestIFTTimeout_RefundUsesFinalizedDestinationAnchor(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t,
		e2etest.EVMRequirements{ControlledMining: true}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	sourceAttestor, err := env.Attestor(meshAttestorFor(route.Source, route.Destination))
	require.NoError(t, err)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	iftApp := e2etest.NewIFT(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	source, err := env.Chain(route.Source)
	require.NoError(t, err)
	sourceEVM, err := source.EVM()
	require.NoError(t, err)
	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	destinationEVM, err := destination.EVM()
	require.NoError(t, err)
	destinationMining, err := destination.Mining()
	require.NoError(t, err)
	sourceMining, err := source.Mining()
	require.NoError(t, err)

	// Keep the destination fixed while the source moves far ahead. Timeout
	// still uses the finalized destination anchor rather than the source height.
	require.NoError(t, relayer.Stop(ctx))
	asymmetricTransfer, err := iftApp.Send(ctx, e2etest.IFTRequest{
		Amount:  big.NewInt(4_000_000),
		Timeout: packetTimeout,
	})
	require.NoError(t, err)
	require.NoError(t, asymmetricTransfer.VerifyBurned(ctx))
	require.NoError(t, asymmetricTransfer.VerifyPending(ctx))
	require.NoError(t, destinationMining.AdvanceTime(ctx, packetTimeoutAdvance))
	require.NoError(t, destinationMining.Mine(ctx, 1))
	destinationHeight, err := destination.Height(ctx)
	require.NoError(t, err)
	destinationAnchor := destinationHeight - 1
	anchorHeader, err := destinationEVM.HeaderByNumber(ctx, new(big.Int).SetUint64(destinationAnchor))
	require.NoError(t, err)
	require.Greater(t, anchorHeader.Time, asymmetricTransfer.TimeoutTimestamp())

	require.NoError(t, destinationMining.WithPaused(ctx, func() error {
		require.NoError(t, sourceMining.Mine(ctx, 50))
		sourceHeight, heightErr := source.Height(ctx)
		require.NoError(t, heightErr)
		pausedDestinationHeight, heightErr := destination.Height(ctx)
		require.NoError(t, heightErr)
		require.GreaterOrEqual(t, sourceHeight, pausedDestinationHeight+40)

		relayer = e2etest.StartRelayer(t, driver, env)
		_, awaitErr := e2etest.AwaitState(ctx, relayer, asymmetricTransfer.PacketTx(),
			relayerv2.PacketState_PACKET_STATE_TIMED_OUT)
		require.NoError(t, awaitErr)
		require.NoError(t, asymmetricTransfer.VerifyRefunded(ctx))
		require.NoError(t, asymmetricTransfer.VerifyNotMinted(ctx))
		require.NoError(t, asymmetricTransfer.VerifyPendingCleared(ctx))
		sourceState := attestedClientState(t, sourceEVM, sourceAttestor.IBCClient().LightClientAddress())
		require.GreaterOrEqual(t, sourceState.LatestHeight, destinationAnchor)
		return nil
	}))
}

func TestIFTTransfer_AutoRelay(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	iftApp := e2etest.NewIFT(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: big.NewInt(1_234_000)})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyBurned(ctx))
	require.NoError(t, transfer.VerifyPending(ctx))

	_, err = e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyPendingCleared(ctx))
	require.NoError(t, transfer.VerifyBurned(ctx), "a successful ack must not also refund")
}

func TestIFTTransfer_TwoTokensSameClientPair(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	tokenA := e2etest.NewIFT(t, env, deployment, sender, route)
	tokenB := e2etest.DeployIFTTokenPair(t, env, deployment, sender, route)
	ctx := t.Context()

	receiver := common.HexToAddress("0xa").Hex()

	transferA, err := tokenA.Send(ctx, e2etest.IFTRequest{
		Amount:   big.NewInt(1_234_000),
		Receiver: receiver,
	})
	require.NoError(t, err)
	transferB, err := tokenB.Send(ctx, e2etest.IFTRequest{
		Amount:   big.NewInt(5_678_000),
		Receiver: receiver,
	})
	require.NoError(t, err)
	require.NotEqual(t, transferA.PacketTx().Sequence, transferB.PacketTx().Sequence)

	transfers := []*e2etest.IFTSend{transferA, transferB}
	for _, transfer := range transfers {
		require.NoError(t, transfer.VerifyBurned(ctx))
		require.NoError(t, transfer.VerifyPending(ctx))
	}

	relayer := e2etest.StartRelayer(t, driver, env)
	require.NoError(t, e2etest.RelayAll(ctx, relayer, transferA.PacketTx()))
	_, err = e2etest.AwaitState(ctx, relayer, transferA.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, transferA.VerifyPendingCleared(ctx))
	require.NoError(t, transferB.VerifyPending(ctx), "token B must remain pending after token A is acknowledged")

	require.NoError(t, e2etest.RelayAll(ctx, relayer, transferB.PacketTx()))
	_, err = e2etest.AwaitState(ctx, relayer, transferB.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	for _, transfer := range transfers {
		require.NoError(t, transfer.VerifyDelivered(ctx))
		require.NoError(t, transfer.VerifyPendingCleared(ctx))
		require.NoError(t, transfer.VerifyBurned(ctx), "a successful ack must not refund either token")
	}
}

func TestIFTTimeout_Refund(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t,
		e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	iftApp := e2etest.NewIFT(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{
		Amount:  big.NewInt(3_000_000),
		Timeout: packetTimeout,
	})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyBurned(ctx))
	require.NoError(t, transfer.VerifyPending(ctx))

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	destinationEVM, err := destination.EVM()
	require.NoError(t, err)
	finalityOffset := uint64(ibclink.HarnessFinalityOffset)
	timeout := time.Unix(int64(transfer.TimeoutTimestamp()), 0) //nolint:gosec // EVM timestamps fit in int64
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		latestHeader, headerErr := destinationEVM.HeaderByNumber(ctx, nil)
		if !assert.NoError(collect, headerErr) ||
			!assert.Greater(collect, latestHeader.Number.Uint64(), finalityOffset) {
			return
		}
		finalizedHeader, headerErr := destinationEVM.HeaderByNumber(
			ctx,
			new(big.Int).SetUint64(latestHeader.Number.Uint64()-finalityOffset),
		)
		if !assert.NoError(collect, headerErr) {
			return
		}
		assert.True(collect, time.Now().After(timeout), "wall clock must pass the packet timeout")
		assert.Greater(collect, finalizedHeader.Time, transfer.TimeoutTimestamp(),
			"finalized destination header must naturally pass the packet timeout")
	}, destination.Timing().CompletionBudget, destination.Timing().PollInterval)

	require.NoError(t, e2etest.RelayAll(ctx, relayer, transfer.PacketTx()))
	_, err = e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_TIMED_OUT)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyRefunded(ctx))
	require.NoError(t, transfer.VerifyNotMinted(ctx))
}

func TestIFTTimeout_WaitsForFinality(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t,
		e2etest.EVMRequirements{ControlledMining: true}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	iftApp := e2etest.NewIFT(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	destinationEVM, err := destination.EVM()
	require.NoError(t, err)
	mining, err := destination.Mining()
	require.NoError(t, err)

	finalityOffset := uint64(ibclink.HarnessFinalityOffset)
	var transfer *e2etest.IFTSend
	require.NoError(t, mining.WithPaused(ctx, func() error {
		transfer, err = iftApp.Send(ctx, e2etest.IFTRequest{
			Amount:  big.NewInt(3_000_000),
			Timeout: packetTimeout,
		})
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyBurned(ctx))
		require.NoError(t, transfer.VerifyPending(ctx))

		timeout := time.Unix(int64(transfer.TimeoutTimestamp()), 0) //nolint:gosec // EVM timestamps fit in int64
		require.Eventually(t, func() bool { return time.Now().After(timeout) },
			destination.Timing().CompletionBudget, destination.Timing().PollInterval,
			"wall clock must pass the packet timeout while destination mining is paused")

		height, heightErr := destination.Height(ctx)
		require.NoError(t, heightErr)
		require.Greater(t, height, finalityOffset)
		finalizedHeader, headerErr := destinationEVM.HeaderByNumber(
			ctx,
			new(big.Int).SetUint64(height-finalityOffset),
		)
		require.NoError(t, headerErr)
		require.Less(t, finalizedHeader.Time, transfer.TimeoutTimestamp(),
			"finalized destination header must remain before the packet timeout")

		require.NoError(t, e2etest.RelayAll(ctx, relayer, transfer.PacketTx()))
		require.NoError(t, e2etest.AwaitStable(ctx, relayer, transfer.PacketTx(),
			relayerv2.PacketState_PACKET_STATE_PENDING))
		require.NoError(t, transfer.VerifyPending(ctx))
		require.NoError(t, transfer.VerifyNotMinted(ctx))
		return nil
	}))

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		height, heightErr := destination.Height(ctx)
		if !assert.NoError(collect, heightErr) ||
			!assert.Greater(collect, height, finalityOffset) {
			return
		}
		finalizedHeader, headerErr := destinationEVM.HeaderByNumber(
			ctx,
			new(big.Int).SetUint64(height-finalityOffset),
		)
		if !assert.NoError(collect, headerErr) {
			return
		}
		assert.Greater(collect, finalizedHeader.Time, transfer.TimeoutTimestamp(),
			"finalized destination header must pass the packet timeout")
	}, destination.Timing().CompletionBudget, destination.Timing().PollInterval)

	_, err = e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_TIMED_OUT)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyRefunded(ctx))
	require.NoError(t, transfer.VerifyNotMinted(ctx))
}

// TestIFTTransfer_ErrorAck_Refund sends to the zero address, which the
// destination mint rejects, forcing an error acknowledgement and a refund.
func TestIFTTransfer_ErrorAck_Refund(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	iftApp := e2etest.NewIFT(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{
		Amount:   big.NewInt(1_234_000),
		Receiver: zeroAddressReceiver,
	})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyBurned(ctx))
	require.NoError(t, transfer.VerifyPending(ctx))

	_, err = e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_REJECTED)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	require.NoError(t, transfer.VerifyRefunded(ctx))
	require.NoError(t, transfer.VerifyPendingCleared(ctx))
}

// TestIFTTransfer_ErrorAck_UnregisteredBridge leaves the destination IFT
// bridge unregistered, so onRecvPacket hits IFTBridgeNotFound.
func TestIFTTransfer_ErrorAck_UnregisteredBridge(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	route.SkipDestinationIFTBridge = true
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	iftApp := e2etest.NewIFT(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: big.NewInt(1_234_000)})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyBurned(ctx))
	require.NoError(t, transfer.VerifyPending(ctx))

	_, err = e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_REJECTED)
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
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	iftApp := e2etest.NewIFT(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	require.NoError(t, relayer.Stop(ctx))
	amounts := []*big.Int{big.NewInt(1_000_000), big.NewInt(2_000_000), big.NewInt(3_000_000)}
	transfers := make([]*e2etest.IFTSend, len(amounts))
	for i, amount := range amounts {
		transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: amount})
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyBurned(ctx))
		require.NoError(t, transfer.VerifyPending(ctx))
		transfers[i] = transfer
	}

	relayer = e2etest.StartRelayer(t, driver, env)
	for _, transfer := range transfers {
		_, err := e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
			relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
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
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	iftApp := e2etest.NewIFT(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	const packetCount = 10
	requests := make([]e2etest.IFTRequest, packetCount)
	for i := range requests {
		requests[i] = e2etest.IFTRequest{Amount: big.NewInt(int64(1_000_000 + i))}
	}
	batch, err := iftApp.SendBatch(ctx, requests)
	require.NoError(t, err)
	packetTxs := batch.PacketTxs()
	require.Len(t, packetTxs, packetCount)

	wantSequences := make(map[uint64]struct{}, packetCount)
	for _, packetTx := range packetTxs {
		wantSequences[packetTx.Sequence] = struct{}{}
	}
	require.Len(t, wantSequences, packetCount, "packets must have distinct sequences")

	require.NoError(t, e2etest.RelayAll(ctx, relayer, packetTxs[0]))

	for _, packetTx := range packetTxs {
		_, err = e2etest.AwaitState(ctx, relayer, packetTx,
			relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
		require.NoError(t, err)
	}
	require.NoError(t, batch.VerifyDelivered(ctx))
	require.NoError(t, batch.VerifyBurned(ctx))

	statuses, err := relayer.PacketStatuses(ctx, string(route.Source), batch.TxHash())
	require.NoError(t, err)
	require.Len(t, statuses, packetCount)
	gotSequences := make(map[uint64]struct{}, packetCount)
	for _, status := range statuses {
		require.Equal(t, relayerv2.PacketState_PACKET_STATE_SUCCEEDED, status.State)
		gotSequences[status.SequenceNumber] = struct{}{}
	}
	require.Equal(t, wantSequences, gotSequences)
}

func TestRelay_FilteredSequences(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t,
		e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	iftApp := e2etest.NewIFT(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	const packetCount = 3
	requests := make([]e2etest.IFTRequest, packetCount)
	for i := range requests {
		requests[i] = e2etest.IFTRequest{Amount: big.NewInt(int64(1_000_000 + i))}
	}
	batch, err := iftApp.SendBatch(ctx, requests)
	require.NoError(t, err)
	packetTxs := batch.PacketTxs()
	require.NoError(t, batch.VerifyBalances(ctx, nil))

	selected := []int{0, 2}
	require.NoError(t, e2etest.RelaySelected(ctx, relayer, packetTxs[0], packetTxs[2]))
	for _, index := range selected {
		_, err = e2etest.AwaitState(ctx, relayer, packetTxs[index],
			relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
		require.NoError(t, err)
	}
	require.NoError(t, e2etest.AwaitStable(ctx, relayer, packetTxs[1],
		relayerv2.PacketState_PACKET_STATE_NOT_SELECTED))
	require.NoError(t, batch.VerifyBalances(ctx, selected))

	require.NoError(t, e2etest.RelaySelected(ctx, relayer, packetTxs[1]))
	_, err = e2etest.AwaitState(ctx, relayer, packetTxs[1],
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, batch.VerifyBalances(ctx, []int{0, 1, 2}))
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
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, sender, relayerSigner, withBatchOverride, route)
	iftApp := e2etest.NewIFT(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	const packetCount = 10
	sends := make([]*e2etest.IFTSend, packetCount)
	for i := range sends {
		transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: big.NewInt(int64(1_000_000 + i))})
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyBurned(ctx))
		sends[i] = transfer
	}

	relayErrs := make([]error, packetCount)
	var wg sync.WaitGroup
	for i, send := range sends {
		wg.Add(1)
		go func(i int, send *e2etest.IFTSend) {
			defer wg.Done()
			relayErrs[i] = e2etest.RelayAll(ctx, relayer, send.PacketTx())
		}(i, send)
	}
	wg.Wait()
	for i, err := range relayErrs {
		require.NoErrorf(t, err, "relay call for packet %d (seq %d) failed", i, sends[i].PacketTx().Sequence)
	}

	for i, send := range sends {
		statuses, err := relayer.PacketStatuses(ctx, string(route.Source), send.TxHash())
		require.NoError(t, err)
		require.Lenf(t, statuses, 1, "packet %d status should only include its own transaction", i)
		require.Equal(t, send.PacketTx().Sequence, statuses[0].SequenceNumber)
	}

	recvTxSequences := make(map[string][]uint64, packetCount)
	ackTxSequences := make(map[string][]uint64, packetCount)
	for _, send := range sends {
		status, err := e2etest.AwaitState(ctx, relayer, send.PacketTx(),
			relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
		require.NoError(t, err)
		require.NoError(t, send.VerifyDelivered(ctx))

		sequence := send.PacketTx().Sequence
		recvHash := status.GetRecvTx().GetTxHash()
		ackHash := status.GetAckTx().GetTxHash()
		recvTxSequences[recvHash] = append(recvTxSequences[recvHash], sequence)
		ackTxSequences[ackHash] = append(ackTxSequences[ackHash], sequence)
	}
	require.NoError(t, sends[packetCount-1].VerifyBurned(ctx), "successful acks must not refund")

	for txHash, want := range recvTxSequences {
		got, err := iftApp.WriteAcknowledgementSequences(ctx, txHash)
		require.NoError(t, err)
		require.ElementsMatchf(t, want, got, "receive transaction %s packet sequences", txHash)
	}
	for txHash, want := range ackTxSequences {
		got, err := iftApp.AckPacketSequences(ctx, txHash)
		require.NoError(t, err)
		require.ElementsMatchf(t, want, got, "acknowledgement transaction %s packet sequences", txHash)
	}
	require.Lessf(t, len(recvTxSequences), packetCount,
		"expected batched recv, got one recv tx per packet: %v", recvTxSequences)
	require.Lessf(t, len(ackTxSequences), packetCount,
		"expected batched ack, got one ack tx per packet: %v", ackTxSequences)
}

// TestIFTTransfer_BatchedTimeout sends more IFT transfers
// with a short packet timeout and asserts they are relayed
// in batches
func TestIFTTransfer_BatchedTimeout(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t,
		e2etest.EVMRequirements{ControlledMining: true}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, sender, relayerSigner, withBatchOverride, route)
	iftApp := e2etest.NewIFT(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	const packetCount = 10
	sends := make([]*e2etest.IFTSend, packetCount)
	for i := range sends {
		transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{
			Amount:  big.NewInt(int64(1_000_000 + i)),
			Timeout: packetTimeout,
		})
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyBurned(ctx))
		sends[i] = transfer
	}

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	mining, err := destination.Mining()
	require.NoError(t, err)
	require.NoError(t, mining.AdvanceTime(ctx, packetTimeoutAdvance))

	relayErrs := make([]error, packetCount)
	var wg sync.WaitGroup
	for i, send := range sends {
		wg.Add(1)
		go func(i int, send *e2etest.IFTSend) {
			defer wg.Done()
			relayErrs[i] = e2etest.RelayAll(ctx, relayer, send.PacketTx())
		}(i, send)
	}
	wg.Wait()
	for i, err := range relayErrs {
		require.NoErrorf(t, err, "relay call for packet %d (seq %d) failed", i, sends[i].PacketTx().Sequence)
	}

	timeoutTxSequences := make(map[string][]uint64, packetCount)
	for _, send := range sends {
		status, err := e2etest.AwaitState(ctx, relayer, send.PacketTx(),
			relayerv2.PacketState_PACKET_STATE_TIMED_OUT)
		require.NoError(t, err)
		require.NoError(t, send.VerifyNotMinted(ctx))

		txHash := status.GetTimeoutTx().GetTxHash()
		timeoutTxSequences[txHash] = append(timeoutTxSequences[txHash], send.PacketTx().Sequence)
	}

	for txHash, want := range timeoutTxSequences {
		got, err := iftApp.TimeoutPacketSequences(ctx, txHash)
		require.NoError(t, err)
		require.ElementsMatchf(t, want, got, "timeout transaction %s packet sequences", txHash)
	}
	require.Lessf(t, len(timeoutTxSequences), packetCount,
		"expected batched timeouts, got one timeout tx per packet: %v", timeoutTxSequences)
}
