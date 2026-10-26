// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"math/big"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
)

func TestTransactionRecovery_ReceiveAfterMempoolDrop(t *testing.T) {
	t.Parallel()

	// Arrange.
	ctx, fixture := setupTransactionRecovery(t, 123456)

	// Act.
	require.NoError(t, fixture.client.Client().CallContext(ctx, nil, "anvil_dropAllTransactions"))
	_, _, err := fixture.client.TransactionByHash(ctx, fixture.original.Hash())
	require.ErrorIs(t, err, ethereum.NotFound)
	require.NoError(t, fixture.transfer.VerifyReceiptAbsent(ctx))
	require.NoError(t, fixture.transfer.VerifyNotMinted(ctx))
	require.NoError(t, fixture.mining.Resume(ctx))

	// Assert.
	status, err := e2etest.AwaitState(
		ctx,
		fixture.relayer,
		fixture.packet,
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED,
	)
	require.NoError(t, err)
	retry, _, err := fixture.client.TransactionByHash(ctx, common.HexToHash(status.GetRecvTx().GetTxHash()))
	require.NoError(t, err)
	require.Equal(t, fixture.original.Nonce(), retry.Nonce())
	require.NoError(t, fixture.transfer.VerifyDelivered(ctx))
	require.NoError(t, fixture.transfer.VerifyCommitmentCleared(ctx))
}

func TestTransactionRecovery_RepricesAfterBaseFeeIncrease(t *testing.T) {
	t.Parallel()

	// Arrange.
	ctx, fixture := setupTransactionRecovery(t, 123456)

	// Act.
	require.NoError(t, fixture.relayer.Kill())

	baseFee := new(big.Int).Mul(fixture.original.GasFeeCap(), big.NewInt(2))
	mine := func() {
		require.NoError(t, fixture.client.Client().CallContext(ctx, nil,
			"anvil_setNextBlockBaseFeePerGas", hexutil.EncodeBig(baseFee)))
		require.NoError(t, fixture.mining.Mine(ctx, 1))
	}
	mine()
	_, err := fixture.client.TransactionReceipt(ctx, fixture.original.Hash())
	require.ErrorIs(t, err, ethereum.NotFound)
	require.NoError(t, fixture.transfer.VerifyNotMinted(ctx))
	fixture.relayer = e2etest.StartRelayer(t, fixture.driver, fixture.env)

	// Assert.
	// Hold the fee above the original cap on every block so the underpriced transaction doesn't get mined.
	blocks := time.NewTicker(fixture.timing.BlockInterval)
	defer blocks.Stop()
	var status *relayerv2.PacketStatus
	for {
		select {
		case <-blocks.C:
			mine()
		case <-ctx.Done():
			t.Fatal("receive did not recover after the base fee increase:", ctx.Err())
		}
		statuses, statusErr := fixture.relayer.PacketStatuses(
			ctx,
			string(fixture.packet.Source),
			fixture.packet.SourceTxHash,
		)
		require.NoError(t, statusErr)
		require.Len(t, statuses, 1)
		status = statuses[0]
		if status.GetState() == relayerv2.PacketState_PACKET_STATE_SUCCEEDED {
			break
		}
	}
	require.NoError(t, fixture.mining.Resume(ctx))

	// Verify the replacement reuses the nonce and raises the fee cap.
	replacement, _, err := fixture.client.TransactionByHash(ctx, common.HexToHash(status.GetRecvTx().GetTxHash()))
	require.NoError(t, err)
	require.NotEqual(t, fixture.original.Hash(), replacement.Hash())
	require.Equal(t, fixture.original.Nonce(), replacement.Nonce())
	require.Positive(t, replacement.GasFeeCap().Cmp(fixture.original.GasFeeCap()))

	// Confirm it was mined successfully at the higher base fee.
	receipt, err := fixture.client.TransactionReceipt(ctx, replacement.Hash())
	require.NoError(t, err)
	require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status)
	header, err := fixture.client.HeaderByNumber(ctx, receipt.BlockNumber)
	require.NoError(t, err)
	require.Equal(t, baseFee, header.BaseFee)

	// Confirm the original stayed unmined and the transfer completed.
	_, err = fixture.client.TransactionReceipt(ctx, fixture.original.Hash())
	require.ErrorIs(t, err, ethereum.NotFound)
	require.NoError(t, fixture.transfer.VerifyDelivered(ctx))
	require.NoError(t, fixture.transfer.VerifyCommitmentCleared(ctx))
}

func TestTransactionRecovery_RepricesPendingReceiveAfterRestart(t *testing.T) {
	t.Parallel()

	// Arrange.
	ctx, fixture := setupTransactionRecovery(t, 123456)
	require.NoError(t, fixture.transfer.VerifyNotMinted(ctx))

	// Act.
	require.NoError(t, fixture.relayer.Kill())
	_, pending, err := fixture.client.TransactionByHash(ctx, fixture.original.Hash())
	require.NoError(t, err)
	require.True(t, pending, "original transaction must remain in the mempool")
	_, err = fixture.client.TransactionReceipt(ctx, fixture.original.Hash())
	require.ErrorIs(t, err, ethereum.NotFound)
	fixture.relayer = e2etest.StartRelayer(t, fixture.driver, fixture.env)

	// Assert.
	// Wait for a replacement at the original nonce to enter the mempool.
	var replacement *types.Transaction
	require.Eventually(t, func() bool {
		statuses, statusErr := fixture.relayer.PacketStatuses(
			ctx,
			string(fixture.packet.Source),
			fixture.packet.SourceTxHash,
		)
		if statusErr != nil || len(statuses) != 1 {
			return false
		}
		hash := common.HexToHash(statuses[0].GetRecvTx().GetTxHash())
		if hash == (common.Hash{}) || hash == fixture.original.Hash() {
			return false
		}
		tx, isPending, txErr := fixture.client.TransactionByHash(ctx, hash)
		if txErr != nil || !isPending || tx.Nonce() != fixture.original.Nonce() {
			return false
		}
		replacement = tx
		return true
	}, fixture.timing.CompletionBudget, fixture.timing.PollInterval,
		"a replacement at the original nonce must enter the mempool")

	// Preserve the nonce, destination, and value while increasing both fees.
	require.Equal(t, fixture.original.Nonce(), replacement.Nonce())
	require.Equal(t, fixture.original.To(), replacement.To())
	require.Equal(t, fixture.original.Value(), replacement.Value())
	require.Positive(t, replacement.GasFeeCap().Cmp(fixture.original.GasFeeCap()))
	require.Positive(t, replacement.GasTipCap().Cmp(fixture.original.GasTipCap()))

	// Mine the replacement and confirm the packet completes.
	require.NoError(t, fixture.mining.Mine(ctx, 1))
	receipt, err := fixture.client.TransactionReceipt(ctx, replacement.Hash())
	require.NoError(t, err)
	require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status)
	require.NoError(t, fixture.mining.Resume(ctx))
	status, err := e2etest.AwaitState(
		ctx,
		fixture.relayer,
		fixture.packet,
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED,
	)
	require.NoError(t, err)
	require.Equal(t, replacement.Hash().Hex(), status.GetRecvTx().GetTxHash())
	require.NoError(t, fixture.transfer.VerifyDelivered(ctx))
	require.NoError(t, fixture.transfer.VerifyCommitmentCleared(ctx))
	_, err = fixture.client.TransactionReceipt(ctx, fixture.original.Hash())
	require.ErrorIs(t, err, ethereum.NotFound)
}

func TestTransactionRecovery_ReorgBeforeFinality(t *testing.T) {
	t.Parallel()

	// Arrange.
	ctx, fixture := setupTransactionRecovery(t, 345678)
	// Mine the receive without a following block, leaving it below the finality requirement.
	require.NoError(t, fixture.mining.Mine(ctx, 1))
	receipt, err := fixture.client.TransactionReceipt(ctx, fixture.original.Hash())
	require.NoError(t, err)
	require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status)
	require.NoError(t, fixture.transfer.VerifyCommitmentPresent(ctx))
	require.NoError(t, fixture.relayer.Kill())

	// Act.
	var reverted bool
	require.NoError(t, fixture.client.Client().CallContext(ctx, &reverted, "evm_revert", fixture.snapshot))
	require.True(t, reverted)
	require.NoError(t, fixture.client.Client().CallContext(ctx, nil, "anvil_dropAllTransactions"))
	// Restore the old height on a different fork without mining the original transaction.
	require.NoError(t, fixture.mining.AdvanceTime(ctx, time.Second))
	_, err = fixture.client.TransactionReceipt(ctx, fixture.original.Hash())
	require.ErrorIs(t, err, ethereum.NotFound)
	require.NoError(t, fixture.transfer.VerifyReceiptAbsent(ctx))
	require.NoError(t, fixture.mining.Resume(ctx))
	fixture.relayer = e2etest.StartRelayer(t, fixture.driver, fixture.env)

	// Assert.
	status, err := e2etest.AwaitState(
		ctx,
		fixture.relayer,
		fixture.packet,
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED,
	)
	require.NoError(t, err)
	require.NoError(t, fixture.transfer.VerifyDelivered(ctx))
	require.NoError(t, fixture.transfer.VerifyCommitmentCleared(ctx))

	// Confirm the receive was mined again at the same nonce in a different block.
	rebroadcast, _, err := fixture.client.TransactionByHash(ctx, common.HexToHash(status.GetRecvTx().GetTxHash()))
	require.NoError(t, err)
	require.Equal(t, fixture.original.Nonce(), rebroadcast.Nonce())
	canonical, err := fixture.client.TransactionReceipt(ctx, rebroadcast.Hash())
	require.NoError(t, err)
	require.Equal(t, types.ReceiptStatusSuccessful, canonical.Status)
	require.NotEqual(t, receipt.BlockHash, canonical.BlockHash)
}

type transactionRecovery struct {
	env      *environment.Environment
	driver   *ibccli.Driver
	mining   *environment.Mining
	client   *ethclient.Client
	timing   environment.Timing
	relayer  *ibccli.Relayer
	transfer *e2etest.TransferSend
	packet   e2etest.PacketTx
	original *types.Transaction
	snapshot string
}

// setupTransactionRecovery deploys a transfer route between two Anvil chains with
// a 5s retry timeout, pauses destination mining, and submits a pending receive.
// It snapshots the destination before submission for the reorg test and registers
// cleanup for the returned context and resources.
func setupTransactionRecovery(t *testing.T, amount int64) (context.Context, *transactionRecovery) {
	t.Helper()
	spec, runtime := e2etest.AttestedMesh(
		e2etest.EVMChains(t, e2etest.EVMRequirements{
			Provider: e2etest.EVMProviderAnvil, ControlledMining: true,
		}, e2etest.ChainA, e2etest.ChainB),
	)
	env := e2etest.Start(t, spec, runtime)
	sender, relayerSigner := e2etest.NewSigner(t), e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, sender, relayerSigner,
		func(cfg *ibccli.RelayerConfig) {
			for i := range cfg.Chains {
				cfg.Chains[i].PendingTxTimeout = 5 * time.Second
			}
		}, route)
	app := e2etest.NewTransfer(t, env, deployment, sender, route)
	destination, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	mining, err := destination.Mining()
	require.NoError(t, err)
	client, err := ethclient.Dial(destination.RPCURL())
	require.NoError(t, err)
	t.Cleanup(client.Close)
	timing := destination.Timing()
	ctx, cancel := context.WithTimeout(t.Context(), 2*timing.CompletionBudget)
	t.Cleanup(cancel)
	require.NoError(t, mining.Pause(ctx))
	// Allow submission through the chain freshness check while mining is paused.
	require.NoError(t, mining.AdvanceTime(ctx, 2*timing.CompletionBudget))
	var snapshot string
	require.NoError(t, client.Client().CallContext(ctx, &snapshot, "evm_snapshot"))
	relayer := e2etest.StartRelayer(t, driver, env)
	transfer, err := app.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(amount)})
	require.NoError(t, err)
	packet := transfer.PacketTx()
	require.NoError(t, e2etest.RelayAll(ctx, relayer, packet))
	var status *relayerv2.PacketStatus
	require.Eventually(t, func() bool {
		statuses, statusErr := relayer.PacketStatuses(ctx, string(packet.Source), packet.SourceTxHash)
		if statusErr != nil || len(statuses) != 1 {
			return false
		}
		status = statuses[0]
		return status.GetRecvTx().GetTxHash() != "" && status.GetState() == relayerv2.PacketState_PACKET_STATE_PENDING
	}, timing.CompletionBudget, timing.PollInterval, "pending receive must be recorded")
	tx, pending, err := client.TransactionByHash(ctx, common.HexToHash(status.GetRecvTx().GetTxHash()))
	require.NoError(t, err)
	require.True(t, pending, "receive transaction must be pending")
	return ctx, &transactionRecovery{
		env: env, driver: driver, mining: mining, client: client, timing: timing,
		relayer: relayer, transfer: transfer, packet: packet, original: tx, snapshot: snapshot,
	}
}
