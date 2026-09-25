// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
)

func TestTransfer_AutoRelay(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	transferApp := e2etest.NewTransfer(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	amount := new(big.Int).Mul(big.NewInt(500_000), big.NewInt(1_000_000_000_000_000_000))
	transfer, err := transferApp.Send(ctx, e2etest.TransferRequest{
		Amount: amount,
		Memo:   "transfer-auto-relay",
	})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyEscrowed(ctx))

	status, err := e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyCommitmentCreated(ctx))
	require.NoError(t, transfer.VerifyReceiptCreated(ctx))
	require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
	require.NoError(t, transfer.VerifyAcknowledgementWritten(ctx, status.GetRecvTx().GetTxHash()))
	require.NoError(t, transfer.VerifyAcknowledgementExecuted(ctx, status.GetAckTx().GetTxHash()))
}

func TestTransfer_ManualRelay(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	transferApp := e2etest.NewTransfer(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	transfer, err := transferApp.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(1_234_000)})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyEscrowed(ctx))

	require.NoError(t, e2etest.AwaitStable(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_PENDING))
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	require.NoError(t, e2etest.RelayAll(ctx, relayer, transfer.PacketTx()))
	_, err = e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
}

const (
	// packetTimeout must elapse in real time before the relayer times a
	// packet out (the pipeline gates on the relayer's own clock), so it has
	// to fit well inside the await budget.
	packetTimeout = 5 * time.Second
	// Advance well past packetTimeout so the packet is decisively expired
	// when the relayer starts.
	packetTimeoutAdvance = 5 * packetTimeout
)

func TestTransferTimeout_Refund(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t,
		e2etest.EVMRequirements{ControlledMining: true}, e2etest.ChainA, e2etest.ChainB))
	requireTimeoutRefund(t, spec, runtime, func(ctx context.Context, destination *environment.Chain) error {
		mining, err := destination.Mining()
		if err != nil {
			return err
		}
		return mining.AdvanceTime(ctx, packetTimeoutAdvance)
	})
}

// A timeout through besu-qbft light clients proves the receipt's absence with
// a non-membership proof. Besu cannot move its clock, so the packet expires as
// the destination keeps producing blocks in real time.
func TestTransferBesuQBFT_TimeoutRefund(t *testing.T) {
	t.Parallel()
	spec, runtime := qbftMesh(e2etest.EVMChains(
		t, e2etest.EVMRequirements{Provider: e2etest.EVMProviderBesu}, e2etest.ChainA, e2etest.ChainB,
	))
	requireTimeoutRefund(t, spec, runtime, awaitDestinationPastTimeout)
}

// requireTimeoutRefund sends a transfer with the relayer stopped, lets expire
// move the destination past its timeout, and requires the restarted relayer
// to time it out and refund the sender.
func requireTimeoutRefund(
	t *testing.T,
	spec environment.Spec,
	runtime environment.Runtime,
	expire func(ctx context.Context, destination *environment.Chain) error,
) {
	t.Helper()
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	transferApp := e2etest.NewTransfer(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	require.NoError(t, relayer.Stop(ctx))
	transfer, err := transferApp.Send(ctx, e2etest.TransferRequest{
		Amount:  big.NewInt(3_000_000),
		Timeout: packetTimeout,
	})
	require.NoError(t, err)

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	require.NoError(t, expire(ctx, destination))
	relayer = e2etest.StartRelayer(t, driver, env)

	status, err := e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_TIMED_OUT)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyRefunded(ctx, status.GetTimeoutTx().GetTxHash()))
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
}

// awaitDestinationPastTimeout waits until the destination head is more than
// packetTimeout past its head now, which is past any timeout set before.
func awaitDestinationPastTimeout(ctx context.Context, destination *environment.Chain) error {
	evm, err := destination.EVM()
	if err != nil {
		return err
	}
	head, err := evm.HeaderByNumber(ctx, nil)
	if err != nil {
		return err
	}
	deadline := head.Time + uint64(packetTimeout/time.Second)
	for head.Time <= deadline {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
		if head, err = evm.HeaderByNumber(ctx, nil); err != nil {
			return err
		}
	}
	return nil
}

// Two Besu QBFT chains relay through besu-qbft light clients: header, account
// and storage proofs instead of attestations. Besu is only available outside
// fast mode, so this skips there.
func TestTransferBesuQBFT_AutoRelay(t *testing.T) {
	t.Parallel()
	spec, runtime := qbftMesh(e2etest.EVMChains(
		t, e2etest.EVMRequirements{Provider: e2etest.EVMProviderBesu}, e2etest.ChainA, e2etest.ChainB,
	))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	transferApp := e2etest.NewTransfer(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	amount := new(big.Int).Mul(big.NewInt(500_000), big.NewInt(1_000_000_000_000_000_000))
	transfer, err := transferApp.Send(ctx, e2etest.TransferRequest{
		Amount: amount,
		Memo:   "transfer-besu-qbft",
	})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyEscrowed(ctx))

	status, err := e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyCommitmentCreated(ctx))
	require.NoError(t, transfer.VerifyReceiptCreated(ctx))
	require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
	require.NoError(t, transfer.VerifyAcknowledgementWritten(ctx, status.GetRecvTx().GetTxHash()))
	require.NoError(t, transfer.VerifyAcknowledgementExecuted(ctx, status.GetAckTx().GetTxHash()))
}
