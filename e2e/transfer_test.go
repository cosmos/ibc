// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"math/big"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/besumsgs"
	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
)

func TestTransfer_AutoRelay(t *testing.T) {
	t.Parallel()
	spec, runtime := e2etest.AttestedMesh(
		e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB),
	)
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
	spec, runtime := e2etest.AttestedMesh(
		e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB),
	)
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
	spec, runtime := e2etest.AttestedMesh(e2etest.EVMChains(t,
		e2etest.EVMRequirements{ControlledMining: true}, e2etest.ChainA, e2etest.ChainB))
	requireTimeoutRefund(t, spec, runtime, advanceDestinationPastTimeout)
}

// A timeout through Besu QBFT light clients proves the receipt's absence with
// a non-membership proof. Besu cannot move its clock, so the packet expires as
// the destination keeps producing blocks in real time.
func TestTransferBesuQBFT_TimeoutRefund(t *testing.T) {
	t.Parallel()
	spec, runtime := e2etest.BesuMesh(e2etest.BesuChains(t, besu.QBFT, besu.QBFT))
	requireTimeoutRefund(t, spec, runtime, awaitDestinationPastTimeout)
}

// requireTimeoutRefund sends a transfer with the relayer stopped, lets expire
// move the destination past the packet's timeout, and requires the restarted
// relayer to time it out and refund the sender.
func requireTimeoutRefund(
	t *testing.T,
	spec environment.Spec,
	runtime environment.Runtime,
	expire func(ctx context.Context, destination *environment.Chain, timeout uint64) error,
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

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	require.NoError(t, relayer.Stop(ctx))
	// The timeout counts from the destination head, which may be a block old,
	// and must outlast the approval and the send landing on the source, each
	// one block plus a possible BFT round change of two.
	transfer, err := transferApp.Send(ctx, e2etest.TransferRequest{
		Amount:  big.NewInt(3_000_000),
		Timeout: 10 * destination.Timing().BlockInterval,
	})
	require.NoError(t, err)
	require.NoError(t, expire(ctx, destination, transfer.TimeoutTimestamp()))
	relayer = e2etest.StartRelayer(t, driver, env)

	status, err := e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_TIMED_OUT)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyRefunded(ctx, status.GetTimeoutTx().GetTxHash()))
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
}

// advanceDestinationPastTimeout moves the destination clock past timeout.
func advanceDestinationPastTimeout(ctx context.Context, destination *environment.Chain, timeout uint64) error {
	mining, err := destination.Mining()
	if err != nil {
		return err
	}
	now, err := destinationTime(ctx, destination)
	if err != nil || now > timeout {
		return err
	}
	return mining.AdvanceTime(ctx, time.Duration(timeout-now+1)*time.Second)
}

// awaitDestinationPastTimeout waits for a destination head past timeout.
func awaitDestinationPastTimeout(ctx context.Context, destination *environment.Chain, timeout uint64) error {
	for {
		now, err := destinationTime(ctx, destination)
		if err != nil || now > timeout {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(destination.Timing().PollInterval):
		}
	}
}

// destinationTime is the destination head's timestamp.
func destinationTime(ctx context.Context, destination *environment.Chain) (uint64, error) {
	evm, err := destination.EVM()
	if err != nil {
		return 0, err
	}
	head, err := evm.HeaderByNumber(ctx, nil)
	if err != nil {
		return 0, err
	}
	return head.Time, nil
}

// Two Besu QBFT chains relay through Besu QBFT light clients: header, account
// and storage proofs instead of attestations. Besu is only available outside
// fast mode, so this skips there.
func TestTransferBesuQBFT_AutoRelay(t *testing.T) {
	t.Parallel()
	requireBesuAutoRelay(t, besu.QBFT)
}

// Two Besu IBFT 2.0 chains relay through the IBFT 2.0 light client, whose
// commit seals sign a different digest than QBFT's.
func TestTransferBesuIBFT2_AutoRelay(t *testing.T) {
	t.Parallel()
	requireBesuAutoRelay(t, besu.IBFT2)
}

// Header voting brings three followers in as validators on chain B and votes
// three of its four genesis validators out before any packet is relayed, so
// only one validator chain A's client trusts seals B's head, short of the two
// its one-half trust level needs. The relayer steps the client across the
// changes in the transaction relaying the transfer.
func TestTransferBesuQBFT_ValidatorTurnover(t *testing.T) {
	t.Parallel()
	requireBesuValidatorTurnover(t, besu.QBFT)
}

// IBFT 2.0 votes on validators through its own RPC namespace and seals over
// its own digest.
func TestTransferBesuIBFT2_ValidatorTurnover(t *testing.T) {
	t.Parallel()
	requireBesuValidatorTurnover(t, besu.IBFT2)
}

func requireBesuValidatorTurnover(t *testing.T, consensus besu.Consensus) {
	t.Helper()
	chains := e2etest.BesuChains(t, consensus, consensus)
	chainB, ok := chains[1].(environment.ManagedBesu)
	require.True(t, ok)
	chainB.Followers = 3
	chains[1] = chainB
	spec, runtime := e2etest.BesuMesh(chains)
	// At the default two thirds, a client cannot follow a removal sealed by a
	// bare quorum of the remaining validators; one half follows every single
	// validator change.
	connection := &spec.Connections[0]
	for _, end := range []*environment.ClientSpec{&connection.A, &connection.B} {
		client, ok := (*end).(environment.NewBesuClient)
		require.True(t, ok)
		client.TrustLevel = besumsgs.IBesuLightClientMsgsTrustThreshold{Numerator: 1, Denominator: 2}
		*end = client
	}
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.BtoA(e2etest.ChainB, e2etest.ChainA)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	transferApp := e2etest.NewTransfer(t, env, deployment, sender, route)
	ctx := t.Context()

	source, err := env.Chain(route.Source)
	require.NoError(t, err)
	validators, err := source.Validators()
	require.NoError(t, err)
	nodes := validators.Nodes()
	for _, follower := range nodes[4:] {
		require.NoError(t, validators.Add(ctx, follower))
	}
	// Node 0 serves the chain's RPC, so it stays a validator.
	for _, genesis := range nodes[1:4] {
		require.NoError(t, validators.Remove(ctx, genesis))
	}
	current, err := validators.Current(ctx)
	require.NoError(t, err)
	require.ElementsMatch(t, slices.Concat(nodes[:1], nodes[4:]), current)

	relayer := e2etest.StartRelayer(t, driver, env)
	transfer, err := transferApp.Send(ctx, e2etest.TransferRequest{
		Amount: big.NewInt(2_000_000),
		Memo:   "turnover-besu-" + string(consensus),
	})
	require.NoError(t, err)
	status, err := e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyAcknowledgementExecuted(ctx, status.GetAckTx().GetTxHash()))
}

// requireBesuAutoRelay relays a transfer between two Besu chains running
// consensus through the light client for that consensus.
func requireBesuAutoRelay(t *testing.T, consensus besu.Consensus) {
	t.Helper()
	spec, runtime := e2etest.BesuMesh(e2etest.BesuChains(t, consensus, consensus))
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
		Memo:   "transfer-besu-" + string(consensus),
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
