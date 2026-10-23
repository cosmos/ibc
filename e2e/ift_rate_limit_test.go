// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/ibcmulticall"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/ics26router"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/ift"
	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
)

const (
	rateLimitCapacity = 100
	rateLimitAmount   = 60
	// Keep the bucket exhausted throughout observation; refund tests advance chain time to refill it.
	rateLimitWindow = time.Hour
)

func TestIFTTransfer_RecvRateLimitRefund(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := t.Context()
	spec, runtime := e2etest.AttestedMesh(
		e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB),
	)
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.DeployWithRelayerConfig(
		t, env, sender, e2etest.NewSigner(t), withRateLimitBatch, route,
	)
	apps, ok := deployment.Chain(route.Destination)
	require.True(t, ok)
	iftApp := e2etest.NewIFT(t, env, deployment, sender, route)
	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	setIFTRateLimit(t, destination, deployment, sender)

	batch := sendRateLimitBatch(t, iftApp, 0)
	packets := batch.PacketTxs()
	relayer := e2etest.StartRelayer(t, driver, env)

	// ACT
	require.NoError(t, e2etest.RelayAll(ctx, relayer, packets[0]))

	// ASSERT
	// Either packet may be processed first, so wait for one delivery to succeed.
	var statuses []*relayerv2.PacketStatus
	require.Eventually(t, func() bool {
		statuses, err = relayer.PacketStatuses(ctx, string(route.Source), batch.TxHash())
		return err == nil && len(statuses) == 2 &&
			(statuses[0].State == relayerv2.PacketState_PACKET_STATE_SUCCEEDED ||
				statuses[1].State == relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	}, destination.Timing().CompletionBudget, destination.Timing().PollInterval)

	// Match the delivered packet by sequence, independent of status response order.
	settled := statuses[0]
	if settled.State != relayerv2.PacketState_PACKET_STATE_SUCCEEDED {
		settled = statuses[1]
	}
	delivered, refunded := 0, 1
	if settled.SequenceNumber != packets[0].Sequence {
		delivered, refunded = refunded, delivered
	}

	// Check delivery, rejection, and the resulting token balances.
	deliveredStatus, err := e2etest.AwaitState(ctx, relayer, packets[delivered],
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	refundedStatus, err := e2etest.AwaitState(ctx, relayer, packets[refunded],
		relayerv2.PacketState_PACKET_STATE_REJECTED)
	require.NoError(t, err)
	require.NoError(t, batch.VerifyState(ctx, []int{delivered}, []int{refunded}))

	// Both packets must have been processed in the same receive batch.
	recvHash := deliveredStatus.GetRecvTx().GetTxHash()
	require.Equal(t, recvHash, refundedStatus.GetRecvTx().GetTxHash(), "both recvs must execute in one batch")
	client, err := destination.EVM()
	require.NoError(t, err)
	receipt, err := client.TransactionReceipt(ctx, common.HexToHash(recvHash))
	require.NoError(t, err)

	// Both receives write acknowledgements, including the rejected transfer.
	sequences, err := iftApp.WriteAcknowledgementSequences(ctx, recvHash)
	require.NoError(t, err)
	require.ElementsMatch(t, []uint64{packets[0].Sequence, packets[1].Sequence}, sequences)

	// Prove the rejected transfer hit the limit, rather than an unrelated application error.
	parser, err := ics26router.NewContractFilterer(apps.ICS26Router, nil)
	require.NoError(t, err)
	iftABI, err := ift.ContractMetaData.GetAbi()
	require.NoError(t, err)
	var callbackErrors [][]byte
	for _, log := range receipt.Logs {
		if log.Address != apps.ICS26Router {
			continue
		}
		if event, parseErr := parser.ParseIBCAppRecvPacketCallbackError(*log); parseErr == nil {
			callbackErrors = append(callbackErrors, event.Reason)
		}
	}
	rateLimitError := iftABI.Errors["RateLimitExceeded"].ID
	require.Equal(t, [][]byte{rateLimitError[:4]}, callbackErrors)
	require.NoError(
		t,
		e2etest.AwaitStable(ctx, relayer, packets[refunded], relayerv2.PacketState_PACKET_STATE_REJECTED),
	)
}

func TestIFTTimeout_RateLimitRetry(t *testing.T) {
	t.Parallel()
	testIFTRefundRateLimit(t, packetTimeout)
}

func TestIFTErrorAck_RateLimitRetry(t *testing.T) {
	t.Parallel()
	testIFTRefundRateLimit(t, 0)
}

func testIFTRefundRateLimit(t *testing.T, timeout time.Duration) {
	t.Helper()

	// ARRANGE
	ctx := t.Context()
	spec, runtime := e2etest.AttestedMesh(e2etest.EVMChains(t,
		e2etest.EVMRequirements{ControlledMining: true}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	// An unregistered bridge makes both receives return error acknowledgements.
	route.SkipDestinationIFTBridge = timeout == 0
	source, err := env.Chain(route.Source)
	require.NoError(t, err)
	driver, deployment := e2etest.DeployWithRelayerConfig(
		t, env, sender, e2etest.NewSigner(t), withRateLimitBatch, route,
	)
	apps, ok := deployment.Chain(route.Source)
	require.True(t, ok)
	iftApp := e2etest.NewIFT(t, env, deployment, sender, route)
	batch := sendRateLimitBatch(t, iftApp, timeout)
	packets := batch.PacketTxs()
	// Tighten the source limit after sending so only refunds, not outbound burns, are constrained.
	setIFTRateLimit(t, source, deployment, sender)
	terminal := relayerv2.PacketState_PACKET_STATE_REJECTED
	sequencesInTx := iftApp.AckPacketSequences
	refundTx := (*relayerv2.PacketStatus).GetAckTx
	if timeout != 0 {
		terminal = relayerv2.PacketState_PACKET_STATE_TIMED_OUT
		sequencesInTx = iftApp.TimeoutPacketSequences
		refundTx = (*relayerv2.PacketStatus).GetTimeoutTx
		requireTimeoutFinalized(t, env, route.Destination, batch.LatestTimeoutTimestamp())
	}
	relayer := e2etest.StartRelayer(t, driver, env)

	// ACT #1: relay both refunds with capacity for only one.
	require.NoError(t, e2etest.RelayAll(ctx, relayer, packets[0]))

	// ASSERT #1: one refund succeeds; the other remains pending.
	// Either packet may be processed first, so wait for one refund to settle.
	var statuses []*relayerv2.PacketStatus
	require.Eventually(t, func() bool {
		statuses, err = relayer.PacketStatuses(ctx, string(route.Source), batch.TxHash())
		return err == nil && len(statuses) == 2 && (statuses[0].State == terminal || statuses[1].State == terminal)
	}, source.Timing().CompletionBudget, source.Timing().PollInterval)

	// Match by sequence because status responses may be in a different order.
	settled := statuses[0]
	if settled.State != terminal {
		settled = statuses[1]
	}
	refunded, pending := 0, 1
	if settled.SequenceNumber != packets[0].Sequence {
		refunded, pending = pending, refunded
	}
	refundedStatus, err := e2etest.AwaitState(ctx, relayer, packets[refunded], terminal)
	require.NoError(t, err)
	_, err = e2etest.AwaitState(ctx, relayer, packets[pending], relayerv2.PacketState_PACKET_STATE_PENDING)
	require.NoError(t, err)

	// Check that the batch's failed subcall reverted with RateLimitExceeded.
	firstHash := refundTx(refundedStatus).GetTxHash()
	client, err := source.EVM()
	require.NoError(t, err)
	receipt, err := client.TransactionReceipt(ctx, common.HexToHash(firstHash))
	require.NoError(t, err)
	parser, err := ibcmulticall.NewContractFilterer(apps.IBCMulticall, nil)
	require.NoError(t, err)
	var failures [][]byte
	for _, log := range receipt.Logs {
		if log.Address != apps.IBCMulticall {
			continue
		}
		event, parseErr := parser.ParseCallFailed(*log)
		require.NoError(t, parseErr)
		failures = append(failures, event.ReturnData)
	}
	iftABI, abiErr := ift.ContractMetaData.GetAbi()
	require.NoError(t, abiErr)
	limitError := iftABI.Errors["RateLimitExceeded"].ID
	require.Equal(t, [][]byte{limitError[:4]}, failures)

	// Only the successful refund should emit an ack or timeout event.
	sequences, err := sequencesInTx(ctx, firstHash)
	require.NoError(t, err)
	require.Equal(t, []uint64{packets[refunded].Sequence}, sequences,
		"only the refund that fits may succeed")

	// The blocked refund stays pending, and balances reflect just one refund.
	require.NoError(t, e2etest.AwaitStable(ctx, relayer, packets[pending], relayerv2.PacketState_PACKET_STATE_PENDING))
	require.NoError(t, batch.VerifyState(ctx, nil, []int{refunded}))

	// ACT #2: refill the bucket without another Relay request.
	mining, err := source.Mining()
	require.NoError(t, err)
	require.NoError(t, mining.AdvanceTime(ctx, rateLimitWindow))

	// ASSERT #2: only the outstanding refund is retried.
	retried, err := e2etest.AwaitState(ctx, relayer, packets[pending], terminal)
	require.NoError(t, err)
	retryHash := refundTx(retried).GetTxHash()
	require.NotEqual(t, firstHash, retryHash)
	sequences, err = sequencesInTx(ctx, retryHash)
	require.NoError(t, err)
	require.Equal(t, []uint64{packets[pending].Sequence}, sequences)
	_, err = e2etest.AwaitState(ctx, relayer, packets[refunded], terminal)
	require.NoError(t, err)
	require.NoError(t, batch.VerifyState(ctx, nil, []int{0, 1}))
}

func sendRateLimitBatch(t *testing.T, app *e2etest.IFT, timeout time.Duration) *e2etest.IFTBatch {
	t.Helper()
	batch, err := app.SendBatch(t.Context(), []e2etest.IFTRequest{
		{Amount: big.NewInt(rateLimitAmount), Timeout: timeout},
		{Amount: big.NewInt(rateLimitAmount), Timeout: timeout},
	})
	require.NoError(t, err)
	require.Len(t, batch.PacketTxs(), 2)
	require.NoError(t, batch.VerifyState(t.Context(), nil, nil))
	return batch
}

func withRateLimitBatch(cfg *ibccli.RelayerConfig) {
	for index := range cfg.Chains {
		cfg.Chains[index].PacketBatchSize = 2
		cfg.Chains[index].PacketBatchTimeout = batchTestPacketBatchTimeout
	}
}

func setIFTRateLimit(t *testing.T, chain *environment.Chain, deployment *e2etest.Deployment, sender e2etest.Signer) {
	t.Helper()
	apps, ok := deployment.Chain(chain.ID())
	require.True(t, ok)
	client, err := chain.EVM()
	require.NoError(t, err)
	iftABI, err := ift.ContractMetaData.GetAbi()
	require.NoError(t, err)
	data, err := iftABI.Pack(
		"setIFTRateLimit",
		big.NewInt(rateLimitCapacity),
		big.NewInt(int64(rateLimitWindow/time.Second)),
	)
	require.NoError(t, err)
	_, err = sender.BroadcastTx(t.Context(), client, apps.IFT, data)
	require.NoError(t, err)
}
