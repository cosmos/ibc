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
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
)

func TestTransactionRecovery_ReceiveAfterMempoolDrop(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(
		e2etest.EVMChains(t, e2etest.EVMRequirements{
			Provider: e2etest.EVMProviderAnvil, ControlledMining: true,
		}, e2etest.ChainA, e2etest.ChainB),
	)
	env := e2etest.Start(t, spec, runtime)
	destination, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	mining, err := destination.Mining()
	require.NoError(t, err)
	sender, relayerSigner := e2etest.NewSigner(t), e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, sender, relayerSigner,
		func(cfg *ibccli.RelayerConfig) {
			for i := range cfg.Chains {
				cfg.Chains[i].PendingTxTimeout = 5 * time.Second
			}
		}, route)
	app := e2etest.NewTransfer(t, env, deployment, sender, route)
	ctx, cancel := context.WithTimeout(t.Context(), 2*destination.Timing().CompletionBudget)
	defer cancel()
	require.NoError(t, mining.Pause(ctx))
	// Allow submission through the chain freshness check while mining is paused.
	require.NoError(t, mining.AdvanceTime(ctx, 2*destination.Timing().CompletionBudget))
	relayer := e2etest.StartRelayer(t, driver, env)
	transfer, err := app.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(123456)})
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
		return status.GetRecvTx().GetTxHash() != ""
	}, destination.Timing().CompletionBudget, destination.Timing().PollInterval,
		"receive hash must be recorded before clearing the mempool")
	client, err := ethclient.Dial(destination.RPCURL())
	require.NoError(t, err)
	t.Cleanup(client.Close)
	original, pending, err := client.TransactionByHash(ctx, common.HexToHash(status.GetRecvTx().GetTxHash()))
	require.NoError(t, err)
	require.True(t, pending)
	require.NoError(t, client.Client().CallContext(ctx, nil, "anvil_dropAllTransactions"))
	_, _, err = client.TransactionByHash(ctx, original.Hash())
	require.ErrorIs(t, err, ethereum.NotFound)
	require.NoError(t, transfer.VerifyReceiptAbsent(ctx))
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	require.NoError(t, mining.Resume(ctx))
	status, err = e2etest.AwaitState(ctx, relayer, packet, relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	retry, _, err := client.TransactionByHash(ctx, common.HexToHash(status.GetRecvTx().GetTxHash()))
	require.NoError(t, err)
	require.Equal(t, original.Nonce(), retry.Nonce())
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
}

func TestTransactionRecovery_RepricesAfterBaseFeeIncrease(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(
		e2etest.EVMChains(t, e2etest.EVMRequirements{
			Provider: e2etest.EVMProviderAnvil, ControlledMining: true,
		}, e2etest.ChainA, e2etest.ChainB),
	)
	env := e2etest.Start(t, spec, runtime)
	destination, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	mining, err := destination.Mining()
	require.NoError(t, err)
	sender, relayerSigner := e2etest.NewSigner(t), e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, sender, relayerSigner,
		func(cfg *ibccli.RelayerConfig) {
			for i := range cfg.Chains {
				cfg.Chains[i].PendingTxTimeout = 5 * time.Second
			}
		}, route)
	app := e2etest.NewTransfer(t, env, deployment, sender, route)
	ctx, cancel := context.WithTimeout(t.Context(), 2*destination.Timing().CompletionBudget)
	defer cancel()
	require.NoError(t, mining.Pause(ctx))
	// Allow the first broadcast through the chain freshness check while mining is paused.
	require.NoError(t, mining.AdvanceTime(ctx, 2*destination.Timing().CompletionBudget))
	relayer := e2etest.StartRelayer(t, driver, env)
	transfer, err := app.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(123456)})
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
		return status.GetRecvTx().GetTxHash() != ""
	}, destination.Timing().CompletionBudget, destination.Timing().PollInterval,
		"receive hash must be recorded before restarting")
	client, err := ethclient.Dial(destination.RPCURL())
	require.NoError(t, err)
	t.Cleanup(client.Close)
	original, pending, err := client.TransactionByHash(ctx, common.HexToHash(status.GetRecvTx().GetTxHash()))
	require.NoError(t, err)
	require.True(t, pending)
	require.NoError(t, relayer.Kill())

	baseFee := new(big.Int).Mul(original.GasFeeCap(), big.NewInt(2))
	mine := func() {
		require.NoError(t, client.Client().CallContext(ctx, nil,
			"anvil_setNextBlockBaseFeePerGas", hexutil.EncodeBig(baseFee)))
		require.NoError(t, mining.Mine(ctx, 1))
	}
	mine()
	_, err = client.TransactionReceipt(ctx, original.Hash())
	require.ErrorIs(t, err, ethereum.NotFound)
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	relayer = e2etest.StartRelayer(t, driver, env)

	// Hold the fee above the original cap on every block so falling fees cannot rescue it.
	blocks := time.NewTicker(destination.Timing().BlockInterval)
	defer blocks.Stop()
	for status.GetState() != relayerv2.PacketState_PACKET_STATE_SUCCEEDED {
		select {
		case <-blocks.C:
			mine()
		case <-ctx.Done():
			t.Fatal("receive did not recover after the base fee increase:", ctx.Err())
		}
		statuses, statusErr := relayer.PacketStatuses(ctx, string(packet.Source), packet.SourceTxHash)
		require.NoError(t, statusErr)
		require.Len(t, statuses, 1)
		status = statuses[0]
	}
	require.NoError(t, mining.Resume(ctx))
	replacement, _, err := client.TransactionByHash(ctx, common.HexToHash(status.GetRecvTx().GetTxHash()))
	require.NoError(t, err)
	require.NotEqual(t, original.Hash(), replacement.Hash())
	require.Equal(t, original.Nonce(), replacement.Nonce())
	require.Positive(t, replacement.GasFeeCap().Cmp(original.GasFeeCap()))
	receipt, err := client.TransactionReceipt(ctx, replacement.Hash())
	require.NoError(t, err)
	require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status)
	header, err := client.HeaderByNumber(ctx, receipt.BlockNumber)
	require.NoError(t, err)
	require.Equal(t, baseFee, header.BaseFee)
	_, err = client.TransactionReceipt(ctx, original.Hash())
	require.ErrorIs(t, err, ethereum.NotFound)
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
}

func TestTransactionRecovery_RepricesPendingReceiveAfterRestart(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(
		e2etest.EVMChains(t, e2etest.EVMRequirements{
			Provider: e2etest.EVMProviderAnvil, ControlledMining: true,
		}, e2etest.ChainA, e2etest.ChainB),
	)
	env := e2etest.Start(t, spec, runtime)
	destination, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	mining, err := destination.Mining()
	require.NoError(t, err)
	sender, relayerSigner := e2etest.NewSigner(t), e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, sender, relayerSigner,
		func(cfg *ibccli.RelayerConfig) {
			for i := range cfg.Chains {
				cfg.Chains[i].PendingTxTimeout = 5 * time.Second
			}
		}, route)
	app := e2etest.NewTransfer(t, env, deployment, sender, route)
	ctx, cancel := context.WithTimeout(t.Context(), 2*destination.Timing().CompletionBudget)
	defer cancel()
	require.NoError(t, mining.Pause(ctx))
	// Keep the paused chain ahead of the relayer's freshness check until replacement.
	require.NoError(t, mining.AdvanceTime(ctx, 2*destination.Timing().CompletionBudget))
	relayer := e2etest.StartRelayer(t, driver, env)
	transfer, err := app.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(123456)})
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
		return status.GetRecvTx().GetTxHash() != ""
	}, destination.Timing().CompletionBudget, destination.Timing().PollInterval,
		"receive hash must be recorded before restarting")
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	require.NoError(t, relayer.Kill())
	client, err := ethclient.Dial(destination.RPCURL())
	require.NoError(t, err)
	t.Cleanup(client.Close)
	original, pending, err := client.TransactionByHash(ctx, common.HexToHash(status.GetRecvTx().GetTxHash()))
	require.NoError(t, err)
	require.True(t, pending, "original transaction must remain in the mempool")
	_, err = client.TransactionReceipt(ctx, original.Hash())
	require.ErrorIs(t, err, ethereum.NotFound)
	relayer = e2etest.StartRelayer(t, driver, env)

	var replacement *types.Transaction
	require.Eventually(t, func() bool {
		statuses, statusErr := relayer.PacketStatuses(ctx, string(packet.Source), packet.SourceTxHash)
		if statusErr != nil || len(statuses) != 1 {
			return false
		}
		hash := common.HexToHash(statuses[0].GetRecvTx().GetTxHash())
		if hash == (common.Hash{}) || hash == original.Hash() {
			return false
		}
		tx, isPending, txErr := client.TransactionByHash(ctx, hash)
		if txErr != nil || !isPending || tx.Nonce() != original.Nonce() {
			return false
		}
		replacement = tx
		return true
	}, destination.Timing().CompletionBudget, destination.Timing().PollInterval,
		"a replacement at the original nonce must enter the mempool")
	require.Equal(t, original.Nonce(), replacement.Nonce())
	require.Equal(t, original.To(), replacement.To())
	require.Equal(t, original.Value(), replacement.Value())
	require.Positive(t, replacement.GasFeeCap().Cmp(original.GasFeeCap()))
	require.Positive(t, replacement.GasTipCap().Cmp(original.GasTipCap()))
	require.NoError(t, mining.Mine(ctx, 1))
	receipt, err := client.TransactionReceipt(ctx, replacement.Hash())
	require.NoError(t, err)
	require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status)
	require.NoError(t, mining.Resume(ctx))
	status, err = e2etest.AwaitState(ctx, relayer, packet, relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.Equal(t, replacement.Hash().Hex(), status.GetRecvTx().GetTxHash())
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
	_, err = client.TransactionReceipt(ctx, original.Hash())
	require.ErrorIs(t, err, ethereum.NotFound)
}

func TestTransactionRecovery_ReorgBeforeFinality(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(
		e2etest.EVMChains(t, e2etest.EVMRequirements{
			Provider: e2etest.EVMProviderAnvil, ControlledMining: true,
		}, e2etest.ChainA, e2etest.ChainB),
	)
	env := e2etest.Start(t, spec, runtime)
	destination, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	mining, err := destination.Mining()
	require.NoError(t, err)
	client, err := ethclient.Dial(destination.RPCURL())
	require.NoError(t, err)
	t.Cleanup(client.Close)
	sender, relayerSigner := e2etest.NewSigner(t), e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, sender, relayerSigner,
		func(cfg *ibccli.RelayerConfig) {
			for i := range cfg.Chains {
				cfg.Chains[i].PendingTxTimeout = 5 * time.Second
			}
		}, route)
	app := e2etest.NewTransfer(t, env, deployment, sender, route)
	ctx, cancel := context.WithTimeout(t.Context(), 2*destination.Timing().CompletionBudget)
	defer cancel()
	require.NoError(t, mining.Pause(ctx))
	// Allow submission through the chain freshness check while mining is paused.
	require.NoError(t, mining.AdvanceTime(ctx, 2*destination.Timing().CompletionBudget))
	var snapshot string
	require.NoError(t, client.Client().CallContext(ctx, &snapshot, "evm_snapshot"))
	relayer := e2etest.StartRelayer(t, driver, env)
	transfer, err := app.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(345678)})
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
	}, destination.Timing().CompletionBudget, destination.Timing().PollInterval,
		"receive must be recorded before the reorg")
	original, pending, err := client.TransactionByHash(ctx, common.HexToHash(status.GetRecvTx().GetTxHash()))
	require.NoError(t, err)
	require.True(t, pending)
	// Mine the receive without a following block, leaving it below the finality requirement.
	require.NoError(t, mining.Mine(ctx, 1))
	receipt, err := client.TransactionReceipt(ctx, original.Hash())
	require.NoError(t, err)
	require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status)
	require.NoError(t, transfer.VerifyCommitmentPresent(ctx))
	require.NoError(t, relayer.Kill())

	var reverted bool
	require.NoError(t, client.Client().CallContext(ctx, &reverted, "evm_revert", snapshot))
	require.True(t, reverted)
	require.NoError(t, client.Client().CallContext(ctx, nil, "anvil_dropAllTransactions"))
	// Restore the old height on a different fork without mining the original transaction.
	require.NoError(t, mining.AdvanceTime(ctx, time.Second))
	_, err = client.TransactionReceipt(ctx, original.Hash())
	require.ErrorIs(t, err, ethereum.NotFound)
	require.NoError(t, transfer.VerifyReceiptAbsent(ctx))
	require.NoError(t, mining.Resume(ctx))
	relayer = e2etest.StartRelayer(t, driver, env)
	status, err = e2etest.AwaitState(ctx, relayer, packet, relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
	rebroadcast, _, err := client.TransactionByHash(ctx, common.HexToHash(status.GetRecvTx().GetTxHash()))
	require.NoError(t, err)
	require.Equal(t, original.Nonce(), rebroadcast.Nonce())
	canonical, err := client.TransactionReceipt(ctx, rebroadcast.Hash())
	require.NoError(t, err)
	require.Equal(t, types.ReceiptStatusSuccessful, canonical.Status)
	require.NotEqual(t, receipt.BlockHash, canonical.BlockHash)
}
